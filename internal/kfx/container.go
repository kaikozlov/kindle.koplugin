package kfx

func isLinkContainerProperty(prop string) bool {
	if heritableProperties[prop] && !reverseHeritablePropertiesExcludes[prop] {
		return true
	}
	switch prop {
	case "-kfx-attrib-colspan", "-kfx-attrib-rowspan", "-kfx-table-vertical-align",
		"-kfx-box-align", "-kfx-heading-level", "-kfx-layout-hints",
		"-kfx-link-color", "-kfx-visited-color":
		return true
	}
	return false
}

func createContainer(contentElem *htmlElement, contentStyle map[string]string, tag string, isContainerProperty func(string) bool) (*htmlElement, map[string]string) {
	containerElem := &htmlElement{Tag: tag, Attrs: map[string]string{}}
	containerElem.Children = []htmlPart{contentElem}

	containerStyle := map[string]string{}
	for prop, val := range contentStyle {
		if isContainerProperty(prop) {
			containerStyle[prop] = val
			delete(contentStyle, prop)
		}
	}

	if styleName, ok := contentStyle["-kfx-style-name"]; ok {
		containerStyle["-kfx-style-name"] = styleName
	}

	return containerElem, containerStyle
}

func createSpanSubcontainer(contentElem *htmlElement, contentStyle map[string]string) *htmlElement {
	subcontainerElem := &htmlElement{Tag: "span", Attrs: map[string]string{}}

	subcontainerElem.Children = append(subcontainerElem.Children, contentElem.Children...)
	contentElem.Children = []htmlPart{subcontainerElem}

	if styleName, ok := contentStyle["-kfx-style-name"]; ok {
		if subcontainerElem.Attrs == nil {
			subcontainerElem.Attrs = map[string]string{}
		}
		subcontainerElem.Attrs["style"] = "-kfx-style-name: " + styleName
	}

	return subcontainerElem
}

func fixVerticalAlignProperties(contentElem *htmlElement, contentStyle map[string]string) map[string]string {
	for _, prop := range []string{"-kfx-baseline-shift", "-kfx-baseline-style", "-kfx-table-vertical-align"} {
		outerVerticalAlign, ok := contentStyle[prop]
		if !ok {
			continue
		}
		delete(contentStyle, prop)

		existingVA, hasVA := contentStyle["vertical-align"]
		if !hasVA {
			contentStyle["vertical-align"] = outerVerticalAlign
		} else if existingVA != outerVerticalAlign {
			subcontainerElem := createSpanSubcontainer(contentElem, contentStyle)
			if subcontainerElem.Attrs == nil {
				subcontainerElem.Attrs = map[string]string{}
			}
			subcontainerElem.Attrs["style"] = "vertical-align: " + existingVA
			delete(contentStyle, "vertical-align")
			contentStyle["vertical-align"] = outerVerticalAlign
		}
	}

	return contentStyle
}
