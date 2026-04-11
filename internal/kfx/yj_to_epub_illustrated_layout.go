package kfx

import (
	"net/url"
	"path"
	"strings"
)

// Port of KFX_EPUB_Illustrated_Layout.fixup_illustrated_layout_anchors (yj_to_epub_illustrated_layout.py L29+).
// Rewrites -kfx-amzn-condition inline styles from anchor: URIs to same-file fragment ids when applicable.
func fixupIllustratedLayoutAnchors(book *decodedBook, sections []renderedSection) {
	if book == nil || !book.IllustratedLayout {
		return
	}
	for i := range sections {
		if sections[i].Root == nil {
			continue
		}
		fixupIllustratedLayoutParts(sections[i].Root.Children, sections[i].Filename)
	}
}

func fixupIllustratedLayoutParts(parts []htmlPart, sectionFilename string) {
	for _, p := range parts {
		el, ok := p.(*htmlElement)
		if !ok || el == nil {
			continue
		}
		if el.Tag == "div" {
			if style, ok := el.Attrs["style"]; ok && strings.Contains(style, "-kfx-amzn-condition") {
				if next := rewriteAmznConditionStyle(style, sectionFilename); next != style {
					el.Attrs["style"] = next
				}
			}
		}
		fixupIllustratedLayoutParts(el.Children, sectionFilename)
	}
}

func rewriteAmznConditionStyle(style string, sectionFilename string) string {
	decls := strings.Split(style, ";")
	changed := false
	for i, decl := range decls {
		decl = strings.TrimSpace(decl)
		if decl == "" {
			continue
		}
		name, value, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name != "-kfx-amzn-condition" {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) < 2 {
			continue
		}
		oper := fields[0]
		href := fields[1]
		u, err := url.Parse(href)
		if err != nil || u.Fragment == "" {
			continue
		}
		pathPart := u.Path
		if u.Scheme == "anchor" && u.Opaque != "" {
			pathPart = u.Opaque
		}
		base := path.Base(pathPart)
		if base == "." || base == "/" {
			base = ""
		}
		secBase := strings.TrimSuffix(sectionFilename, path.Ext(sectionFilename))
		otherBase := strings.TrimSuffix(base, path.Ext(base))
		if base != "" && base != sectionFilename && otherBase != secBase {
			continue
		}
		operPrefix := oper
		if idx := strings.Index(oper, "."); idx >= 0 {
			operPrefix = oper[:idx]
		}
		newVal := operPrefix + " " + u.Fragment
		if newVal != value {
			decls[i] = "-kfx-amzn-condition: " + newVal
			changed = true
		}
	}
	if !changed {
		return style
	}
	out := make([]string, 0, len(decls))
	for _, d := range decls {
		d = strings.TrimSpace(d)
		if d != "" {
			out = append(out, d)
		}
	}
	return strings.Join(out, "; ")
}
