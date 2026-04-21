package kfx

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type storylineRenderer struct {
	contentFragments    map[string][]string
	rubyGroups          map[string]map[string]interface{}
	rubyContents        map[string]map[string]interface{}
	resourceHrefByID    map[string]string
	resourceFragments   map[string]resourceFragment
	anchorToFilename    map[string]string
	directAnchorURI     map[string]string
	fallbackAnchorURI   map[string]string
	positionToSection   map[int]string
	positionAnchors     map[int]map[int][]string
	positionAnchorID    map[int]map[int]string
	anchorNamesByID     map[string][]string
	anchorHeadingLevel  map[string]int
	emittedAnchorIDs    map[string]bool
	styleFragments      map[string]map[string]interface{}
	styles              *styleCatalog
	activeBodyClass     string
	activeBodyDefaults  map[string]bool
	firstVisibleSeen    bool
	lastKFXHeadingLevel int
	symFmt              symType
	conditionEvaluator  conditionEvaluator
	resolveResource     ResourceResolver
}

type conditionEvaluator struct {
	orientationLock   string
	fixedLayout       bool
	illustratedLayout bool
}

func (r *storylineRenderer) renderStoryline(sectionPositionID int, bodyStyleID string, bodyStyleValues map[string]interface{}, storyline map[string]interface{}, nodes []interface{}) renderedStoryline {
	result := renderedStoryline{}
	contentNodes := nodes
	promotedBody := false
	inferredBody := false
	if bodyStyleID == "" {
		if promotedStyleID, promotedNodes, ok := promotedBodyContainer(nodes); ok {
			bodyStyleID = promotedStyleID
			bodyStyleValues = nil
			contentNodes = promotedNodes
			promotedBody = true
		}
	}
	if promotedBody {
		bodyStyleValues = mergeStyleValues(bodyStyleValues, r.inferPromotedBodyStyle(contentNodes))
	}
	if bodyStyleID == "" && len(bodyStyleValues) == 0 {
		bodyStyleValues = r.inferBodyStyleValues(contentNodes, defaultInheritedBodyStyle())
		inferredBody = true
		if len(bodyStyleValues) == 0 {
			bodyStyleValues = map[string]interface{}{
				"$11": defaultInheritedBodyStyle()["$11"],
			}
		}
	}
	if os.Getenv("KFX_DEBUG_BODY") != "" {
		fmt.Fprintf(os.Stderr, "body infer styleID=%s values=%#v\n", bodyStyleID, bodyStyleValues)
	}
	r.activeBodyDefaults = nil
	r.firstVisibleSeen = false
	r.lastKFXHeadingLevel = 1
	if bodyStyleID == "" {
		bodyStyleID, _ = asString(storyline["$157"])
	}
	bodyStyle := effectiveStyle(r.styleFragments[bodyStyleID], bodyStyleValues)
	// In Python, text-indent is NOT set on the body element during rendering. Instead,
	// it's promoted to the body by reverse inheritance during simplify_styles.
	// Go previously stripped $36 here, but that prevented text-indent from appearing
	// in child classes (the inherited default "0" matched children's "0", so simplify
	// stripped it). Now we keep text-indent in the body style and let simplify_styles'
	// reverse inheritance handle it — matching Python's approach.
	// delete(bodyStyle, "$36")
	bodyDeclarations := cssDeclarationsFromMap(processContentProperties(bodyStyle, r.resolveResource))
	if bodyStyleID == "" && len(bodyDeclarations) == 0 {
		bodyStyleValues = map[string]interface{}{
			"$11": defaultInheritedBodyStyle()["$11"],
		}
		bodyStyle = effectiveStyle(r.styleFragments[bodyStyleID], bodyStyleValues)
		bodyDeclarations = cssDeclarationsFromMap(processContentProperties(bodyStyle, r.resolveResource))
	}
	if len(bodyDeclarations) > 0 {
		baseName := "class"
		if bodyStyleID != "" {
			baseName = r.styleBaseName(bodyStyleID)
		}
		result.BodyStyle = styleStringFromDeclarations(baseName, nil, bodyDeclarations)
	}
	if os.Getenv("KFX_DEBUG_BODY") != "" {
		fmt.Fprintf(os.Stderr, "body resolved styleID=%s decls=%v style=%s inferred=%v\n", bodyStyleID, bodyDeclarations, result.BodyStyle, inferredBody)
	}
	result.BodyStyleInferred = inferredBody
	if len(bodyDeclarations) > 0 {
		r.activeBodyDefaults = inheritedDefaultSet(bodyDeclarations)
	}
	bodyParts := make([]htmlPart, 0, len(contentNodes))
	for _, node := range contentNodes {
		rendered := r.renderNode(node, 0)
		if rendered != nil {
			bodyParts = append(bodyParts, rendered)
		}
	}
	root := &htmlElement{Attrs: map[string]string{}, Children: bodyParts}
	normalizeHTMLWhitespace(root)
	r.applyPositionAnchors(root, sectionPositionID, false)
	result.Root = root
	result.BodyHTML = renderHTMLParts(root.Children, true)
	if strings.Contains(result.BodyHTML, "<svg ") {
		result.Properties = "svg"
	}
	return result
}

func (r *storylineRenderer) promoteCommonChildStyles(element *htmlElement) {
	if element == nil {
		return
	}
	for _, child := range element.Children {
		if childElement, ok := child.(*htmlElement); ok {
			r.promoteCommonChildStyles(childElement)
		}
	}
	if element.Tag != "div" {
		return
	}
	baseName, parentStyle, ok := r.dynamicClassStyle(element.Attrs["class"])
	if !ok {
		return
	}
	children := make([]*htmlElement, 0, len(element.Children))
	for _, child := range element.Children {
		childElement, ok := child.(*htmlElement)
		if !ok {
			continue
		}
		children = append(children, childElement)
	}
	if len(children) == 0 {
		return
	}
	const reverseInheritanceFraction = 0.8
	keys := []string{"font-family", "font-style", "font-weight", "font-variant", "text-align", "text-indent", "text-transform"}
	valueCounts := map[string]map[string]int{}
	for _, child := range children {
		_, childStyle, ok := r.dynamicClassStyle(child.Attrs["class"])
		if !ok {
			continue
		}
		for _, key := range keys {
			value := childStyle[key]
			if value == "" {
				continue
			}
			if valueCounts[key] == nil {
				valueCounts[key] = map[string]int{}
			}
			valueCounts[key][value]++
		}
	}
	newHeritable := map[string]string{}
	for _, key := range keys {
		values := valueCounts[key]
		if len(values) == 0 {
			continue
		}
		total := 0
		mostCommonValue := ""
		mostCommonCount := 0
		for value, count := range values {
			total += count
			if count > mostCommonCount {
				mostCommonValue = value
				mostCommonCount = count
			}
		}
		if total < len(children) && parentStyle[key] == "" {
			continue
		}
		if float64(mostCommonCount) >= float64(len(children))*reverseInheritanceFraction {
			newHeritable[key] = mostCommonValue
		}
	}
	if len(newHeritable) == 0 {
		return
	}
	oldParentStyle := cloneStyleMap(parentStyle)
	for _, child := range children {
		childBaseName, childStyle, ok := r.dynamicClassStyle(child.Attrs["class"])
		if !ok {
			continue
		}
		for key, newValue := range newHeritable {
			if childStyle[key] == newValue {
				delete(childStyle, key)
			} else if childStyle[key] == "" && oldParentStyle[key] != "" && oldParentStyle[key] != newValue {
				childStyle[key] = oldParentStyle[key]
			}
		}
		r.setDynamicClassStyle(child, childBaseName, childStyle)
	}
	for key, value := range newHeritable {
		parentStyle[key] = value
	}
	r.setDynamicClassStyle(element, baseName, parentStyle)
}

func (r *storylineRenderer) dynamicClassStyle(className string) (string, map[string]string, bool) {
	if r == nil || className == "" || r.styles == nil {
		return "", nil, false
	}
	entry, ok := r.styles.byToken[className]
	if !ok {
		return "", nil, false
	}
	return entry.baseName, parseDeclarationString(entry.declarations), true
}

func (r *storylineRenderer) styleBaseName(styleID string) string {
	if styleID == "" {
		return "class"
	}
	simplified := uniquePartOfLocalSymbol(styleID, r.symFmt)
	if simplified == "" {
		return "class"
	}
	return "class_" + simplified
}

func (r *storylineRenderer) setDynamicClassStyle(element *htmlElement, baseName string, style map[string]string) {
	if element == nil {
		return
	}
	if len(style) == 0 {
		delete(element.Attrs, "class")
		return
	}
	declarations := declarationListFromStyleMap(style)
	if len(declarations) == 0 {
		delete(element.Attrs, "class")
		return
	}
	element.Attrs["class"] = r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) setDynamicStyle(element *htmlElement, baseName string, layoutHints []string, declarations []string) {
	if element == nil {
		return
	}
	setElementStyleString(element, mergeStyleStrings(element.Attrs["style"], styleStringFromDeclarations(baseName, layoutHints, declarations)))
}

func (r *storylineRenderer) renderNode(raw interface{}, depth int) htmlPart {
	node, ok := asMap(raw)
	if !ok {
		// IonString entries in $146 lists create text nodes.
		// Python process_content (yj_to_epub_content.py:397-399) wraps them in <span>.
		if text, ok := asString(raw); ok && text != "" {
			return &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: text}}}
		}
		return nil
	}
	node, ok = r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
	switch asStringDefault(node["$159"]) {
	case "$276":
		if list := r.renderListNode(node, depth); list != nil {
			return r.wrapNodeLink(node, list)
		}
	case "$277":
		if item := r.renderListItemNode(node, depth); item != nil {
			return r.wrapNodeLink(node, item)
		}
	case "$596":
		if rule := r.renderRuleNode(node); rule != nil {
			return r.wrapNodeLink(node, rule)
		}
	case "$439":
		if hidden := r.renderHiddenNode(node, depth); hidden != nil {
			return r.wrapNodeLink(node, hidden)
		}
	case "$278":
		if table := r.renderTableNode(node, depth); table != nil {
			return r.wrapNodeLink(node, table)
		}
	case "$270":
		if container := r.renderFittedContainer(node, depth); container != nil {
			return r.wrapNodeLink(node, container)
		}
	case "$272":
		if svg := r.renderSVGNode(node); svg != nil {
			return r.wrapNodeLink(node, svg)
		}
	case "$274":
		if plugin := r.renderPluginNode(node); plugin != nil {
			return r.wrapNodeLink(node, plugin)
		}
	case "$454":
		if tbody := r.renderStructuredContainer(node, "tbody", depth); tbody != nil {
			return r.wrapNodeLink(node, tbody)
		}
	case "$151":
		if thead := r.renderStructuredContainer(node, "thead", depth); thead != nil {
			return r.wrapNodeLink(node, thead)
		}
	case "$455":
		if tfoot := r.renderStructuredContainer(node, "tfoot", depth); tfoot != nil {
			return r.wrapNodeLink(node, tfoot)
		}
	case "$279":
		if row := r.renderTableRow(node, depth); row != nil {
			return r.wrapNodeLink(node, row)
		}
	}

	if imageNode := r.renderImageNode(node); imageNode != nil {
		return r.wrapNodeLink(node, imageNode)
	}

	if textNode := r.renderTextNode(node, depth); textNode != nil {
		return r.wrapNodeLink(node, textNode)
	}

	children, ok := asSlice(node["$146"])
	if !ok {
		if hasRenderableContainer(node) {
			element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
			if styleAttr := r.containerClass(node); styleAttr != "" {
				element.Attrs["style"] = styleAttr
			}
			r.applyStructuralNodeAttrs(element, node, "")
			return r.wrapNodeLink(node, element)
		}
		return nil
	}

	if inline := r.renderInlineRenderContainer(node, children, depth); inline != nil {
		return r.wrapNodeLink(node, inline)
	}
	// Python defers heading conversion to simplify_styles (yj_to_epub_properties.py:1922).
	// We create a <div> here and store the heading level as a data attribute.
	// simplify_styles will convert it to <h1>-<h6> after seeing all children.
	hl := r.layoutHintHeadingLevel(node, children)
	if hl > 0 {
		element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
		for _, child := range children {
			if inline := r.renderInlineContent(child, depth+1); inline != nil {
				element.Children = append(element.Children, inline)
			}
		}
		if len(element.Children) > 0 {
			if styleAttr := r.containerClass(node); styleAttr != "" {
				element.Attrs["style"] = styleAttr
			}
			// Store heading level for simplify_styles to read. Python reads this from
			// sty.pop("-kfx-heading-level", self.last_kfx_heading_level) (line 1858).
			element.Attrs["data-kfx-heading-level"] = fmt.Sprintf("%d", hl)
			r.applyStructuralNodeAttrs(element, node, "")
			if positionID, _ := asInt(node["$155"]); positionID != 0 {
				r.applyPositionAnchors(element, positionID, false)
			}
			return r.wrapNodeLink(node, element)
		}
	}
	if figure := r.renderFigureHintContainer(node, children, depth); figure != nil {
		return r.wrapNodeLink(node, figure)
	}
	if paragraph := r.renderInlineParagraphContainer(node, children, depth); paragraph != nil {
		return r.wrapNodeLink(node, paragraph)
	}

	container := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	for _, child := range children {
		rendered := r.renderContentChild(child, depth+1, node)
		if rendered != nil {
			container.Children = append(container.Children, rendered)
		}
	}
	// Python yj_to_epub_content.py:1112+: $142 style events are applied to the
	// content element after all children are added. For containers with both element
	// and text children (like the Elvis logo: <img/> + "FIRST EDITION"), the style events
	// wrap text ranges in styled spans. applyContainerStyleEvents implements this.
	r.applyContainerStyleEvents(node, container)
	if len(container.Children) == 0 {
		return nil
	}
	// Python's COMBINE_NESTED_DIVS: if the container wraps a single image wrapper div
	// (<div><img/></div>), merge them into one div. The image wrapper (from imageClasses)
	// already partitioned properties. containerClass includes properties promoted from
	// children via inferPromotedStyleValues.
	// Python: content_style.update(child_sty, replace=False) — parent keeps its values,
	// child only adds properties not already present. So container overwrites wrapper.
	if wrapper := singleImageWrapperChild(container); wrapper != nil {
		containerStyle := r.containerClass(node)
		wrapperStyle := ""
		if wrapper.Attrs != nil {
			wrapperStyle = wrapper.Attrs["style"]
		}
		// mergeStyleStrings processes in order: first arg's properties can be overwritten
		// by second arg. We want container (parent) to win, so wrapper goes first.
		mergedStyle := mergeStyleStrings(wrapperStyle, containerStyle)
		if mergedStyle != "" {
			wrapper.Attrs["style"] = mergedStyle
		} else {
			delete(wrapper.Attrs, "style")
		}
		r.applyStructuralNodeAttrs(wrapper, node, "")
		if positionID, _ := asInt(node["$155"]); positionID != 0 {
			r.applyPositionAnchors(wrapper, positionID, false)
		}
		return r.wrapNodeLink(node, wrapper)
	}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		container.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(container, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(container, positionID, false)
	}
	return r.wrapNodeLink(node, container)
}

func singleImageWrapperChild(container *htmlElement) *htmlElement {
	if len(container.Children) != 1 {
		return nil
	}
	div, ok := container.Children[0].(*htmlElement)
	if !ok || div.Tag != "div" {
		return nil
	}
	if len(div.Children) != 1 {
		return nil
	}
	img, ok := div.Children[0].(*htmlElement)
	if !ok || img.Tag != "img" {
		return nil
	}
	return div
}

func (r *storylineRenderer) renderListNode(node map[string]interface{}, depth int) htmlPart {
	tag := listTagByMarker[asStringDefault(node["$100"])]
	if tag == "" {
		tag = "ul"
	}
	list := &htmlElement{Tag: tag, Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		list.Attrs["style"] = styleAttr
	}
	if start, ok := asInt(node["$104"]); ok && start > 0 && tag == "ol" && start != 1 {
		list.Attrs["start"] = strconv.Itoa(start)
	}
	children, _ := asSlice(node["$146"])
	for _, child := range children {
		if rendered := r.renderNode(child, depth+1); rendered != nil {
			list.Children = append(list.Children, rendered)
		}
	}
	if len(list.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(list, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(list, positionID, false)
	}
	return list
}

func (r *storylineRenderer) renderListItemNode(node map[string]interface{}, depth int) htmlPart {
	item := &htmlElement{Tag: "li", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		item.Attrs["style"] = styleAttr
	}
	if value, ok := asInt(node["$104"]); ok && value > 0 {
		item.Attrs["value"] = strconv.Itoa(value)
	}
	if ref, ok := asMap(node["$145"]); ok {
		text := r.resolveText(ref)
		if text != "" {
			item.Children = append(item.Children, r.applyAnnotations(text, node)...)
		}
	} else if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			if rendered := r.renderNode(child, depth+1); rendered != nil {
				item.Children = append(item.Children, rendered)
				continue
			}
			if inline := r.renderInlinePart(child, depth+1); inline != nil {
				item.Children = append(item.Children, inline)
			}
		}
	}
	if len(item.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(item, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(item, positionID, false)
	}
	return item
}

func (r *storylineRenderer) renderRuleNode(node map[string]interface{}) htmlPart {
	rule := &htmlElement{Tag: "hr", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		rule.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(rule, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(rule, positionID, false)
	}
	return rule
}

func (r *storylineRenderer) renderHiddenNode(node map[string]interface{}, depth int) htmlPart {
	element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	if hiddenStyle := styleStringFromDeclarations("class", nil, []string{"display: none"}); hiddenStyle != "" {
		element.Attrs["style"] = mergeStyleStrings(element.Attrs["style"], hiddenStyle)
	}
	if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			if rendered := r.renderNode(child, depth+1); rendered != nil {
				element.Children = append(element.Children, rendered)
				continue
			}
			if inline := r.renderInlinePart(child, depth+1); inline != nil {
				element.Children = append(element.Children, inline)
			}
		}
	}
	if len(element.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderFittedContainer(node map[string]interface{}, depth int) htmlPart {
	fitWidth, _ := asBool(node["$478"])
	if !fitWidth {
		return nil
	}
	outer := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	if styleAttr := r.fittedContainerClass(node); styleAttr != "" {
		outer.Attrs["style"] = styleAttr
	}
	inner := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	children, _ := asSlice(node["$146"])
	for _, child := range children {
		rendered := r.renderNode(child, depth+1)
		if rendered != nil {
			inner.Children = append(inner.Children, rendered)
		}
	}
	if len(inner.Children) == 0 {
		return nil
	}
	styleID, _ := asString(node["$157"])
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	if styleAttr := styleStringFromDeclarations(baseName, nil, []string{"display: inline-block"}); styleAttr != "" {
		inner.Attrs["style"] = styleAttr
	}
	outer.Children = []htmlPart{inner}
	r.applyStructuralNodeAttrs(outer, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(outer, positionID, false)
	}
	return outer
}

func (r *storylineRenderer) renderPluginNode(node map[string]interface{}) htmlPart {
	resourceID, _ := asString(node["$175"])
	if resourceID == "" {
		return nil
	}
	href := r.resourceHrefByID[resourceID]
	if href == "" {
		return nil
	}
	resource := r.resourceFragments[resourceID]
	alt, _ := asString(node["$584"])
	switch {
	case resource.MediaType == "plugin/kfx-html-article" || resource.MediaType == "text/html" || resource.MediaType == "application/xhtml+xml":
		element := &htmlElement{
			Tag:   "iframe",
			Attrs: map[string]string{"src": href},
		}
		if styleAttr := styleStringFromDeclarations("class", nil, []string{
			"border-bottom-style: none",
			"border-left-style: none",
			"border-right-style: none",
			"border-top-style: none",
			"height: 100%",
			"width: 100%",
		}); styleAttr != "" {
			element.Attrs["style"] = styleAttr
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	case strings.HasPrefix(resource.MediaType, "audio/"):
		element := &htmlElement{
			Tag:   "audio",
			Attrs: map[string]string{"src": href, "controls": "controls"},
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	case strings.HasPrefix(resource.MediaType, "video/"):
		element := &htmlElement{
			Tag:   "video",
			Attrs: map[string]string{"src": href, "controls": "controls"},
		}
		if alt != "" {
			element.Attrs["aria-label"] = alt
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	case strings.HasPrefix(resource.MediaType, "image/"):
		return r.renderImageNode(node)
	default:
		element := &htmlElement{
			Tag:   "object",
			Attrs: map[string]string{"data": href},
		}
		if resource.MediaType != "" {
			element.Attrs["type"] = resource.MediaType
		}
		if alt != "" {
			element.Children = []htmlPart{htmlText{Text: alt}}
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	}
}

func (r *storylineRenderer) renderSVGNode(node map[string]interface{}) htmlPart {
	width, hasWidth := asInt(node["$66"])
	height, hasHeight := asInt(node["$67"])
	attrs := map[string]string{
		"version":             "1.1",
		"preserveAspectRatio": "xMidYMid meet",
	}
	if hasWidth && hasHeight && width > 0 && height > 0 {
		attrs["viewBox"] = fmt.Sprintf("0 0 %d %d", width, height)
	}
	element := &htmlElement{
		Tag:   "svg",
		Attrs: attrs,
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderTableNode(node map[string]interface{}, depth int) htmlPart {
	table := &htmlElement{Tag: "table", Attrs: map[string]string{}}
	if styleAttr := r.tableClass(node); styleAttr != "" {
		table.Attrs["style"] = styleAttr
	}
	if cols, ok := asSlice(node["$152"]); ok && len(cols) > 0 {
		colgroup := &htmlElement{Tag: "colgroup", Attrs: map[string]string{}}
		for _, raw := range cols {
			colMap, ok := asMap(raw)
			if !ok {
				continue
			}
			col := &htmlElement{Tag: "col", Attrs: map[string]string{}}
			if span, ok := asInt(colMap["$118"]); ok && span > 1 {
				col.Attrs["span"] = strconv.Itoa(span)
			}
			if styleAttr := r.tableColumnClass(colMap); styleAttr != "" {
				col.Attrs["style"] = styleAttr
			}
			colgroup.Children = append(colgroup.Children, col)
		}
		if len(colgroup.Children) > 0 {
			table.Children = append(table.Children, colgroup)
		}
	}
	if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			rendered := r.renderNode(child, depth+1)
			if rendered != nil {
				if childNode, ok := asMap(child); ok {
					r.applyStructuralAttrsToPart(rendered, childNode, table.Tag)
				}
				table.Children = append(table.Children, rendered)
			}
		}
	}
	if len(table.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(table, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(table, positionID, false)
	}
	return table
}

func (r *storylineRenderer) renderStructuredContainer(node map[string]interface{}, tag string, depth int) htmlPart {
	element := &htmlElement{Tag: tag, Attrs: map[string]string{}}
	if styleAttr := r.structuredContainerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			rendered := r.renderNode(child, depth+1)
			if rendered != nil {
				element.Children = append(element.Children, rendered)
			}
		}
	}
	if len(element.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderTableRow(node map[string]interface{}, depth int) htmlPart {
	row := &htmlElement{Tag: "tr", Attrs: map[string]string{}}
	if styleID, _ := asString(node["$157"]); styleID != "" {
		if styleAttr := r.structuredContainerClass(node); styleAttr != "" {
			row.Attrs["style"] = styleAttr
		}
	}
	children, _ := asSlice(node["$146"])
	for _, child := range children {
		cellNode, ok := asMap(child)
		if !ok {
			continue
		}
		cell := r.renderTableCell(cellNode, depth+1)
		if cell != nil {
			row.Children = append(row.Children, cell)
		}
	}
	if len(row.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(row, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(row, positionID, false)
	}
	return row
}

func (r *storylineRenderer) renderTableCell(node map[string]interface{}, depth int) htmlPart {
	cell := &htmlElement{Tag: "td", Attrs: map[string]string{}}
	// Extract colspan/rowspan.
	// In KFX data, these can be either:
	// 1. Directly on the node as $148/$149 (some fixtures)
	// 2. In the style fragment as property $148/$149, converted to -kfx-attrib-colspan/rowspan
	// Python extracts them via -kfx-attrib-* partition in fixup_styles_and_classes.
	// Go's cssDeclarationsFromMap strips -kfx- prefixed properties, so we also extract
	// from the style fragment here.
	colspanSet := false
	rowspanSet := false
	if colspan, ok := asInt(node["$148"]); ok && colspan > 1 {
		cell.Attrs["colspan"] = strconv.Itoa(colspan)
		colspanSet = true
	}
	if rowspan, ok := asInt(node["$149"]); ok && rowspan > 1 {
		cell.Attrs["rowspan"] = strconv.Itoa(rowspan)
		rowspanSet = true
	}
	if !colspanSet || !rowspanSet {
		if styleID, _ := asString(node["$157"]); styleID != "" {
			effective := effectiveStyle(r.styleFragments[styleID], node)
			cssMap := processContentProperties(effective, r.resolveResource)
			if !colspanSet {
				if v := cssMap["-kfx-attrib-colspan"]; v != "" {
					if n, err := strconv.Atoi(v); err == nil && n > 1 {
						cell.Attrs["colspan"] = v
					}
				}
			}
			if !rowspanSet {
				if v := cssMap["-kfx-attrib-rowspan"]; v != "" {
					if n, err := strconv.Atoi(v); err == nil && n > 1 {
						cell.Attrs["rowspan"] = v
					}
			}
			}
		}
	}

	// Python COMBINE_NESTED_DIVS check: when the cell $269 has a single $269 child with $145
	// text, and the parent and child CSS have no overlapping properties, the nested divs
	// merge into one. The merged result becomes <td>text</td> after retag + span strip.
	// When there IS overlap, no merge happens: the child $269 keeps its own <div> which
	// simplify_styles later promotes to <p>, giving <td><p class="child">text</p></td>.
	merged := r.tableCellCombineNestedDivs(node)

	if styleAttr := r.tableCellClass(node); styleAttr != "" {
		cell.Attrs["style"] = styleAttr
	}
	if ref, ok := asMap(node["$145"]); ok {
		text := r.resolveText(ref)
		if text != "" {
			cell.Children = append(cell.Children, r.applyAnnotations(text, node)...)
		}
	} else if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			childNode, ok := asMap(child)
			if !ok {
				// Python process_content IonString case (line 397-399):
				// bare string → <span>text</span>
				if text, ok := asString(child); ok {
					s := &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: text}}}
					cell.Children = append(cell.Children, s)
				}
				continue
			}
			if ref, ok := asMap(childNode["$145"]); ok && merged {
				// COMBINE_NESTED_DIVS merged: extract text directly into <td>.
				// Python: merge removes inner div, leaving <span>text</span> which
				// epub_output.py later strips, giving bare text in <td>.
				text := r.resolveText(ref)
				if text != "" {
					cell.Children = append(cell.Children, r.applyAnnotations(text, childNode)...)
				}
				// When the merged child has its own position ID with anchors,
				// promote those anchors to the <td>. In Python, these anchors
				// end up on the <td> after simplify_styles and beautify_html.
				if childPosID, _ := asInt(childNode["$155"]); childPosID != 0 {
					if len(r.positionAnchors[childPosID]) > 0 {
						r.applyPositionAnchors(cell, childPosID, false)
					}
				}
				continue
			}
			// No merge: render child as full node. For a $269 with $145,
			// renderTextNode produces <p class="child">text</p>, matching Python's
			// simplify_styles div→p promotion.
			if rendered := r.renderContentChild(child, depth+1); rendered != nil {
				cell.Children = append(cell.Children, rendered)
			}
		}
	}
	r.applyStructuralNodeAttrs(cell, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(cell, positionID, false)
	}
	return cell
}

// renderInlineContent dispatches a $146 list child the same way Python's process_content
// handles IonString vs IonStruct (yj_to_epub_content.py:395-405).
// - IonString (Go string): creates <span>text</span>  (Python line 397-399)
// - IonStruct (Go map): delegates to renderInlinePart
// applyContainerStyleEvents applies $142 style events to a container element's children,
// matching Python's post-add_content style event processing (yj_to_epub_content.py:1112+).
// Python's find_or_create_style_event_element locates text at character offsets within
// the content element and wraps ranges in styled spans. This implementation handles the
// common case where text children (from IonString $146 items) need annotation wrapping.
func (r *storylineRenderer) applyContainerStyleEvents(node map[string]interface{}, container *htmlElement) {
	annotations, ok := asSlice(node["$142"])
	if !ok || len(annotations) == 0 {
		return
	}
	// Build text offset map
	type textRange struct {
		childIdx    int
		startOffset int
		length      int
	}
	var ranges []textRange
	offset := 0
	for i, child := range container.Children {
		elem, ok := child.(*htmlElement)
		if !ok {
			continue
		}
		switch elem.Tag {
		case "img", "svg":
			ranges = append(ranges, textRange{childIdx: i, startOffset: offset, length: 1})
			offset++
		case "span":
			text := htmlElementText(elem)
			ranges = append(ranges, textRange{childIdx: i, startOffset: offset, length: len([]rune(text))})
			offset += len([]rune(text))
		}
	}
	// Apply each annotation to the matching text range
	for _, ann := range annotations {
		annMap, ok := asMap(ann)
		if !ok {
			continue
		}
		eventStart, _ := asInt(annMap["$143"])
		eventLen, _ := asInt(annMap["$144"])
		if eventLen <= 0 {
			continue
		}
		anchorID, _ := asString(annMap["$179"])
		styleID, _ := asString(annMap["$157"])

		// Phase 1: Handle non-span children (img, svg, div wrappers with img children) for $179 link wrapping.
		// Python: if "$179" in style_event: event_elem = replace_element_with_container(event_elem, "a")
		// Python's locate_offset traverses the element tree and can find <img> nested inside
		// wrapper <div>s. Each img/svg counts as 1 character in the offset map.
		if anchorID != "" {
			href := r.anchorHref(anchorID)
			if href != "" {
				for _, tr := range ranges {
					elem := container.Children[tr.childIdx].(*htmlElement)
					if elem.Tag == "span" {
						continue
					}
					annEnd := eventStart + eventLen
					trEnd := tr.startOffset + tr.length
					if eventStart >= trEnd || annEnd <= tr.startOffset {
						continue
					}
					// Find the actual target element.
					// If the direct child is a <div> wrapping an <img>, Python's locate_offset
					// would traverse into the div and find the <img> at the same offset.
					target := elem
					if elem.Tag == "div" {
						if img := findFirstDescendantByTag(elem, "img"); img != nil {
							target = img
						} else if svg := findFirstDescendantByTag(elem, "svg"); svg != nil {
							target = svg
						}
					}
					if target != elem {
						// Target is nested: find target's parent and replace target with <a><target/></a>.
						var findParent func(*htmlElement) *htmlElement
						findParent = func(e *htmlElement) *htmlElement {
							for _, c := range e.Children {
								if ch, ok := c.(*htmlElement); ok {
									if ch == target {
										return e
									}
									if p := findParent(ch); p != nil {
										return p
									}
								}
							}
							return nil
						}
						if p := findParent(elem); p != nil {
							wrapChildInLink(p, target, href)
						}
					} else {
						container.Children[tr.childIdx] = &htmlElement{
							Tag:      "a",
							Attrs:    map[string]string{"href": href},
							Children: []htmlPart{elem},
						}
					}
					break
				}
			}
		}

		// Phase 2: Handle span children for style application.
		if styleID == "" {
			continue
		}
		// Find which text range(s) this annotation covers
		for _, tr := range ranges {
			elem := container.Children[tr.childIdx].(*htmlElement)
			if elem.Tag != "span" {
				continue
			}
			text := htmlElementText(elem)
			runes := []rune(text)
			trEnd := tr.startOffset + tr.length
			annEnd := eventStart + eventLen

			// Check overlap
			if eventStart >= trEnd || annEnd <= tr.startOffset {
				continue
			}

			// Compute local offset within this span's text
			localStart := eventStart - tr.startOffset
			if localStart < 0 {
				localStart = 0
			}
			localEnd := annEnd - tr.startOffset
			if localEnd > len(runes) {
				localEnd = len(runes)
			}

			if localStart >= localEnd {
				continue
			}

			// Get the annotation style
			style := effectiveStyle(r.styleFragments[styleID], annMap)
			cssMap := processContentProperties(style, r.resolveResource)
			declarations := cssDeclarationsFromMap(cssMap)
			if len(declarations) == 0 {
				continue
			}
			baseName := "class"
			if styleID != "" {
				baseName = r.styleBaseName(styleID)
			}
			styledSpanClass := styleStringFromDeclarations(baseName, nil, declarations)

			// Split the span: before + styled + after
			before := string(runes[:localStart])
			styled := string(runes[localStart:localEnd])
			after := string(runes[localEnd:])

			var newChildren []htmlPart
			if before != "" {
				newChildren = append(newChildren, &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: before}}})
			}
			styledSpan := &htmlElement{
				Tag:      "span",
				Attrs:    map[string]string{"style": styledSpanClass},
				Children: []htmlPart{htmlText{Text: styled}},
			}
			newChildren = append(newChildren, styledSpan)
			if after != "" {
				newChildren = append(newChildren, &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: after}}})
			}

			// Replace the single span child with the split children
			container.Children = append(container.Children[:tr.childIdx], append(newChildren, container.Children[tr.childIdx+1:]...)...)
			break // annotation applied, move to next
		}
	}
}

// findFirstDescendantByTag finds the first descendant element with the given tag.
func findFirstDescendantByTag(elem *htmlElement, tag string) *htmlElement {
	for _, child := range elem.Children {
		if ch, ok := child.(*htmlElement); ok {
			if ch.Tag == tag {
				return ch
			}
			if found := findFirstDescendantByTag(ch, tag); found != nil {
				return found
			}
		}
	}
	return nil
}

// wrapChildInLink replaces a child element inside its parent's children list
// with <a href="..."><child/></a>.
func wrapChildInLink(parent *htmlElement, target *htmlElement, href string) {
	for i, child := range parent.Children {
		if ch, ok := child.(*htmlElement); ok && ch == target {
			parent.Children[i] = &htmlElement{
				Tag:      "a",
				Attrs:    map[string]string{"href": href},
				Children: []htmlPart{target},
			}
			return
		}
	}
}

// htmlElementText extracts the combined text content of an htmlElement.
func htmlElementText(elem *htmlElement) string {
	var buf strings.Builder
	for _, child := range elem.Children {
		switch typed := child.(type) {
		case htmlText:
			buf.WriteString(typed.Text)
		case *htmlText:
			buf.WriteString(typed.Text)
		}
	}
	return buf.String()
}

func (r *storylineRenderer) renderInlineContent(child interface{}, depth int) htmlPart {
	if text, ok := asString(child); ok {
		return &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: text}}}
	}
	return r.renderInlinePart(child, depth)
}

// renderContentChild dispatches a $146 list child the same way Python's process_content
// handles IonString vs IonStruct for block-level container paths.
// - IonString (Go string): creates <span>text</span> with parent annotations applied
// - IonStruct (Go map): delegates to renderNode, falls back to renderInlinePart
func (r *storylineRenderer) renderContentChild(child interface{}, depth int, parentNode ...map[string]interface{}) htmlPart {
	if text, ok := asString(child); ok {
		return &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: text}}}
	}
	if rendered := r.renderNode(child, depth); rendered != nil {
		return rendered
	}
	return r.renderInlinePart(child, depth)
}

func (r *storylineRenderer) renderInlinePart(raw interface{}, depth int) htmlPart {
	node, ok := asMap(raw)
	if !ok {
		return nil
	}
	node, ok = r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
	if imageNode := r.renderImageNode(node); imageNode != nil {
		return imageNode
	}
	if ref, ok := asMap(node["$145"]); ok {
		text := r.resolveText(ref)
		if text == "" {
			return nil
		}
		content := r.applyAnnotations(text, node)
		styleID, _ := asString(node["$157"])
		positionID, _ := asInt(node["$155"])
		if styleID == "" && positionID == 0 && len(content) == 1 {
			return content[0]
		}
		element := &htmlElement{Tag: "span", Attrs: map[string]string{}, Children: content}
		if styleAttr := r.spanClass(styleID); styleAttr != "" {
			element.Attrs["style"] = styleAttr
		}
		r.applyStructuralNodeAttrs(element, node, "")
		if positionID != 0 {
			r.applyPositionAnchors(element, positionID, false)
		}
		return element
	}
	children, ok := asSlice(node["$146"])
	if !ok {
		return nil
	}
	styleID, _ := asString(node["$157"])
	container := &htmlElement{Tag: "span", Attrs: map[string]string{}}
	for _, child := range children {
		if rendered := r.renderInlinePart(child, depth+1); rendered != nil {
			container.Children = append(container.Children, rendered)
		}
	}
	if len(container.Children) == 0 {
		return nil
	}
	if styleAttr := r.inlineContainerClass(styleID, node); styleAttr != "" {
		container.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(container, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(container, positionID, false)
	}
	return container
}

func (r *storylineRenderer) renderImageNode(node map[string]interface{}) htmlPart {
	node, ok := r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
	resourceID, _ := asString(node["$175"])
	if resourceID == "" {
		return nil
	}
	href := r.resourceHrefByID[resourceID]
	if href == "" {
		return nil
	}
	alt, _ := asString(node["$584"])
	image := &htmlElement{
		Tag:   "img",
		Attrs: map[string]string{"src": href, "alt": alt},
	}
	wrapperClass, imageClass := r.imageClasses(node)
	if imageClass != "" {
		image.Attrs["style"] = imageClass
	}
	// Python process_content $283 (inline render) for <img> (yj_to_epub_content.py:1295-1298):
	// render=="$283" adds -kfx-render:inline to style but does NOT create a container wrapper.
	// The wrapper div is only created in the else branch (non-inline render, line 1324-1330).
	// Without this check, inline images get a wrapper <div> which causes containsBlock=true
	// in simplify_styles, preventing <div>→<p> promotion for containers with mixed image+text.
	renderMode, _ := asString(node["$601"])
	isInlineRender := renderMode == "$283"
	if wrapperClass == "" || isInlineRender {
		firstVisible := r.consumeVisibleElement()
		r.applyStructuralNodeAttrs(image, node, "")
		if positionID, _ := asInt(node["$155"]); positionID != 0 {
			r.applyPositionAnchors(image, positionID, firstVisible)
		}
		return image
	}
	wrapper := &htmlElement{
		Tag:      "div",
		Attrs:    map[string]string{"style": wrapperClass},
		Children: []htmlPart{image},
	}
	r.applyStructuralNodeAttrs(wrapper, node, "")
	firstVisible := r.consumeVisibleElement()
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(wrapper, positionID, firstVisible)
	}
	return wrapper
}

func (r *storylineRenderer) renderTextNode(node map[string]interface{}, depth int) htmlPart {
	_ = depth
	var ok bool
	node, ok = r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
	ref, ok := asMap(node["$145"])
	if !ok {
		return nil
	}
	text := r.resolveText(ref)
	if text == "" {
		return nil
	}
	positionID, _ := asInt(node["$155"])
	if os.Getenv("KFX_DEBUG") != "" && (positionID == 1110 || positionID == 1111 || positionID == 1177 || positionID == 1178) {
		fmt.Fprintf(os.Stderr, "render text pos=%d text=%q style=%s\n", positionID, text[:minInt(len(text), 32)], asStringDefault(node["$157"]))
	}
	content := r.applyAnnotations(text, node)
	annotationStyleID := fullParagraphAnnotationStyleID(node, text)

	styleID, _ := asString(node["$157"])
	level := headingLevel(node)
	if level == 0 {
		level = r.headingLevelForPosition(positionID, 0)
	}
	// Port of Python simplify_styles heading tag selection (yj_to_epub_properties.py ~L1928):
	// Only promote to <h1>-<h6> when layout hints include "heading" AND not fixed/illustrated layout.
	// Python: "heading" in kfx_layout_hints and not contains_block_elem → elem.tag = "h" + level
	isHeading := layoutHintsInclude(r.nodeLayoutHints(node), "heading")
	if level > 0 {
		r.lastKFXHeadingLevel = level
		if !isHeading {
			// Heading level stored in CSS ($790) but layout hints don't confirm heading;
			// render as <p> like Python does (simplify_styles won't promote this <div>).
			level = 0
		}
	} else if isHeading {
		level = r.lastKFXHeadingLevel
	}
	if level > 0 {
		firstVisible := r.consumeVisibleElement()
		element := &htmlElement{
			Tag:      fmt.Sprintf("h%d", level),
			Attrs:    map[string]string{},
			Children: content,
		}
		if styleID != "" {
			if styleAttr := r.headingClass(styleID); styleAttr != "" {
				element.Attrs["style"] = styleAttr
			}
		}
		r.applyStructuralNodeAttrs(element, node, "")
		r.applyPositionAnchors(element, positionID, firstVisible)
		return element
	}

	firstVisible := r.consumeVisibleElement()
	element := &htmlElement{
		Tag:      "p",
		Attrs:    map[string]string{},
		Children: content,
	}
	if styleID != "" {
		if styleAttr := r.paragraphClass(styleID, annotationStyleID); styleAttr != "" {
			element.Attrs["style"] = styleAttr
		}
	}
	r.applyFirstLineStyle(element, node)
	r.applyStructuralNodeAttrs(element, node, "")
	r.applyPositionAnchors(element, positionID, firstVisible)
	return element
}

func removeSingleFullTextLinkClass(parts []htmlPart) {
	if len(parts) != 1 {
		return
	}
	link, ok := parts[0].(*htmlElement)
	if !ok || link == nil || link.Tag != "a" {
		return
	}
	delete(link.Attrs, "class")
	delete(link.Attrs, "style")
}

func (r *storylineRenderer) applyStructuralAttrsToPart(part htmlPart, node map[string]interface{}, parentTag string) {
	element, ok := part.(*htmlElement)
	if !ok {
		return
	}
	r.applyStructuralNodeAttrs(element, node, parentTag)
}

func (r *storylineRenderer) applyFirstLineStyle(element *htmlElement, node map[string]interface{}) {
	if r == nil || element == nil || node == nil {
		return
	}
	raw, ok := asMap(node["$622"])
	if !ok {
		return
	}
	style := cloneMap(raw)
	if styleID, _ := asString(style["$173"]); styleID != "" {
		style = effectiveStyle(r.styleFragments[styleID], style)
	}
	delete(style, "$173")
	delete(style, "$625")
	declarations := cssDeclarationsFromMap(processContentProperties(style, r.resolveResource))
	if len(declarations) == 0 {
		return
	}
	className := r.styles.reserveClass("kfx-firstline")
	if className == "" {
		return
	}
	element.Attrs["class"] = appendClassNames(element.Attrs["class"], className)
	r.styles.addStatic("."+className+"::first-line", declarations)
}

func (r *storylineRenderer) wrapNodeLink(node map[string]interface{}, part htmlPart) htmlPart {
	if node == nil || part == nil {
		return part
	}
	anchorID, _ := asString(node["$179"])
	if anchorID == "" {
		return part
	}
	href := r.anchorHref(anchorID)
	if href == "" {
		return part
	}
	if element, ok := part.(*htmlElement); ok && element != nil && element.Tag == "a" {
		if element.Attrs == nil {
			element.Attrs = map[string]string{}
		}
		if element.Attrs["href"] == "" {
			element.Attrs["href"] = href
		}
		return element
	}
	return &htmlElement{
		Tag:      "a",
		Attrs:    map[string]string{"href": href},
		Children: []htmlPart{part},
	}
}

func (r *storylineRenderer) anchorHref(anchorID string) string {
	if anchorID == "" {
		return ""
	}
	if href := r.directAnchorURI[anchorID]; href != "" {
		return href
	}
	if href := r.anchorToFilename[anchorID]; href != "" {
		return href
	}
	if r.anchorNameRegistered(anchorID) {
		return "anchor:" + anchorID
	}
	return anchorID
}

func (r *storylineRenderer) anchorNameRegistered(anchorID string) bool {
	if r == nil || anchorID == "" {
		return false
	}
	for _, offsets := range r.positionAnchors {
		for _, names := range offsets {
			for _, name := range names {
				if name == anchorID {
					return true
				}
			}
		}
	}
	return false
}

func (r *storylineRenderer) prepareRenderableNode(node map[string]interface{}) (map[string]interface{}, bool) {
	if node == nil {
		return nil, false
	}
	working := cloneMap(node)
	hadConditionalContent := working["$592"] != nil || working["$591"] != nil || working["$663"] != nil
	if include := working["$592"]; include != nil && !r.conditionEvaluator.evaluateBinary(include) {
		return nil, false
	}
	delete(working, "$592")
	if exclude := working["$591"]; exclude != nil && r.conditionEvaluator.evaluateBinary(exclude) {
		return nil, false
	}
	delete(working, "$591")
	if rawConditional, ok := asSlice(working["$663"]); ok {
		for _, raw := range rawConditional {
			props, ok := asMap(raw)
			if !ok {
				continue
			}
			if merged := r.mergeConditionalProperties(working, props); merged != nil {
				working = merged
			}
		}
	}
	delete(working, "$663")
	if hadConditionalContent {
		working["__has_conditional_content__"] = true
	}
	return working, true
}

func (r *storylineRenderer) mergeConditionalProperties(node map[string]interface{}, conditional map[string]interface{}) map[string]interface{} {
	if node == nil || conditional == nil {
		return node
	}
	props := cloneMap(conditional)
	apply := false
	if include := props["$592"]; include != nil {
		apply = r.conditionEvaluator.evaluateBinary(include)
		delete(props, "$592")
	} else if exclude := props["$591"]; exclude != nil {
		apply = !r.conditionEvaluator.evaluateBinary(exclude)
		delete(props, "$591")
	}
	if !apply {
		return node
	}
	merged := cloneMap(node)
	for key, value := range props {
		merged[key] = value
	}
	return merged
}

func (r *storylineRenderer) applyStructuralNodeAttrs(element *htmlElement, node map[string]interface{}, parentTag string) {
	if element == nil || node == nil {
		return
	}
	if element.Tag == "div" {
		if r.shouldPromoteLayoutHints() && layoutHintsInclude(r.nodeLayoutHints(node), "figure") && htmlPartContainsImage(element) {
			element.Tag = "figure"
		}
	}
	classification, _ := asString(node["$615"])
	switch {
	case classification == "$453" && parentTag == "table" && element.Tag == "div":
		element.Tag = "caption"
	case classificationEPUBType[classification] != "" && element.Tag == "div":
		element.Tag = "aside"
	}
	if epubType := classificationEPUBType[classification]; epubType != "" && element.Tag == "aside" {
		element.Attrs["epub:type"] = epubType
	}
	if classification == "$688" {
		element.Attrs["role"] = "math"
	}
	switch asStringDefault(node["$156"]) {
	case "$324", "$325":
		if styleAttr := styleStringFromDeclarations("class", nil, []string{"position: fixed"}); styleAttr != "" {
			element.Attrs["style"] = mergeStyleStrings(element.Attrs["style"], styleAttr)
		}
	}
}

func (r *storylineRenderer) nodeLayoutHints(node map[string]interface{}) []string {
	if node == nil {
		return nil
	}
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	switch typed := style["$761"].(type) {
	case string:
		if typed == "" {
			return nil
		}
		if hint := layoutHintElementNames[typed]; hint != "" {
			return []string{hint}
		}
		return strings.Fields(typed)
	case []interface{}:
		hints := make([]string, 0, len(typed))
		for _, raw := range typed {
			value, ok := asString(raw)
			if !ok || value == "" {
				continue
			}
			if hint := layoutHintElementNames[value]; hint != "" {
				hints = append(hints, hint)
				continue
			}
			hints = append(hints, strings.Fields(value)...)
		}
		if len(hints) == 0 {
			return nil
		}
		return hints
	default:
		return nil
	}
}

// layoutHintHeadingLevel returns the heading level (1-6) if the node should become a heading,
// or 0 if it should not. Previously this returned an HTML tag like "h1" and the caller
// created that tag directly. Now we defer the tag decision to simplify_styles (matching
// Python yj_to_epub_properties.py:1922) and only communicate the heading level.
func (r *storylineRenderer) layoutHintHeadingLevel(node map[string]interface{}, children []interface{}) int {
	if !r.shouldPromoteStructuralContainer(node) {
		return 0
	}
	if !layoutHintsInclude(r.nodeLayoutHints(node), "heading") {
		return 0
	}
	level := headingLevel(node)
	if level <= 0 || level > 6 {
		return 0
	}
	for _, child := range children {
		if r.renderInlinePart(child, 0) == nil {
			return 0
		}
	}
	return level
}

// renderInlineParagraphContainer creates a <div> for inline-only containers with text.
// Python creates a <div> via process_content and simplify_styles later converts to <p>.
// Previously Go created <p> here directly — now deferred to simplify_styles (Python parity).
func (r *storylineRenderer) renderInlineParagraphContainer(node map[string]interface{}, children []interface{}, depth int) htmlPart {
	if !r.shouldPromoteStructuralContainer(node) || len(children) != 1 || !nodeContainsTextContent(children) {
		return nil
	}
	element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	for _, child := range children {
		if inline := r.renderInlinePart(child, depth+1); inline != nil {
			element.Children = append(element.Children, inline)
			continue
		}
		return nil
	}
	if len(element.Children) == 0 {
		return nil
	}
	styleID, _ := asString(node["$157"])
	if styleAttr := r.paragraphClass(styleID, ""); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	r.applyFirstLineStyle(element, node)
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderInlineRenderContainer(node map[string]interface{}, children []interface{}, depth int) htmlPart {
	renderMode, _ := asString(node["$601"])
	if renderMode != "$283" {
		return nil
	}
	styleID, _ := asString(node["$157"])
	element := &htmlElement{Tag: "span", Attrs: map[string]string{}}
	for _, child := range children {
		if inline := r.renderInlinePart(child, depth+1); inline != nil {
			element.Children = append(element.Children, inline)
			continue
		}
		return nil
	}
	if len(element.Children) == 0 {
		return nil
	}
	if styleAttr := r.inlineContainerClass(styleID, node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderFigureHintContainer(node map[string]interface{}, children []interface{}, depth int) htmlPart {
	if !r.shouldPromoteStructuralContainer(node) || !layoutHintsInclude(r.nodeLayoutHints(node), "figure") {
		return nil
	}
	element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	for _, child := range children {
		if rendered := r.renderNode(child, depth+1); rendered != nil {
			element.Children = append(element.Children, rendered)
			continue
		}
		if inline := r.renderInlinePart(child, depth+1); inline != nil {
			element.Children = append(element.Children, inline)
			continue
		}
		return nil
	}
	if len(element.Children) == 0 || !htmlPartContainsImage(element) {
		return nil
	}
	// Python's COMBINE_NESTED_DIVS for figure containers.
	if wrapper := singleImageWrapperChild(element); wrapper != nil {
		containerStyle := r.containerClass(node)
		wrapperStyle := ""
		if wrapper.Attrs != nil {
			wrapperStyle = wrapper.Attrs["style"]
		}
		// Parent (container) wins on conflicts, matching Python's replace=False
		mergedStyle := mergeStyleStrings(wrapperStyle, containerStyle)
		if mergedStyle != "" {
			wrapper.Attrs["style"] = mergedStyle
		} else {
			delete(wrapper.Attrs, "style")
		}
		r.applyStructuralNodeAttrs(wrapper, node, "")
		if positionID, _ := asInt(node["$155"]); positionID != 0 {
			r.applyPositionAnchors(wrapper, positionID, false)
		}
		return wrapper
	}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) shouldPromoteLayoutHints() bool {
	if r == nil {
		return true
	}
	return !r.conditionEvaluator.fixedLayout && !r.conditionEvaluator.illustratedLayout
}

func (r *storylineRenderer) shouldPromoteStructuralContainer(node map[string]interface{}) bool {
	if !r.shouldPromoteLayoutHints() || node == nil {
		return false
	}
	if node["__has_conditional_content__"] != nil || node["$615"] != nil {
		return false
	}
	switch asStringDefault(node["$156"]) {
	case "$324", "$325":
		return false
	}
	return true
}

func nodeContainsTextContent(children []interface{}) bool {
	for _, raw := range children {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		if _, ok := asMap(node["$145"]); ok {
			return true
		}
		if nested, ok := asSlice(node["$146"]); ok && nodeContainsTextContent(nested) {
			return true
		}
	}
	return false
}

func layoutHintsInclude(hints []string, want string) bool {
	for _, hint := range hints {
		if hint == want {
			return true
		}
	}
	return false
}

func htmlPartContainsImage(part htmlPart) bool {
	switch typed := part.(type) {
	case *htmlElement:
		if typed == nil {
			return false
		}
		if typed.Tag == "img" {
			return true
		}
		for _, child := range typed.Children {
			if htmlPartContainsImage(child) {
				return true
			}
		}
	}
	return false
}

func (r *storylineRenderer) bodyClass(styleID string, values map[string]interface{}) string {
	style := effectiveStyle(r.styleFragments[styleID], values)
	if len(style) == 0 {
		return ""
	}
	declarations := cssDeclarationsFromMap(processContentProperties(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	if bodyClass := staticBodyClassForDeclarations(declarations); bodyClass != "" {
		return bodyClass
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) containerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	if len(style) == 0 {
		return ""
	}
	cssMap := processContentProperties(style, r.resolveResource)

	// Handle -kfx-box-align → margin auto conversion, matching Python
	// yj_to_epub_content.py:1390-1404. Container elements get margin-auto
	// only when they have a width property (or are tables).
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		delete(cssMap, "-kfx-box-align")
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" {
			_, hasWidth := cssMap["width"]
			if hasWidth {
				if boxAlign != "left" {
					cssMap["margin-left"] = "auto"
				}
				if boxAlign != "right" {
					cssMap["margin-right"] = "auto"
				}
			}
		}
	}

	declarations := cssDeclarationsFromMap(cssMap)
	if mapFontStyle(style["$12"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, r.nodeLayoutHints(node), declarations)
}

func (r *storylineRenderer) tableClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	cssMap := processContentProperties(style, r.resolveResource)

	// Handle -kfx-box-align → margin auto conversion for tables.
	// Ported from Python yj_to_epub_content.py (~L1390-1404):
	// For tables with box-align left/right/center, set the appropriate
	// margin-left/margin-right to auto (replacing any explicit value).
	// Tables always have a known width, so auto margins are appropriate.
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		delete(cssMap, "-kfx-box-align")
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" {
			if boxAlign != "left" {
				cssMap["margin-left"] = "auto"
			}
			if boxAlign != "right" {
				cssMap["margin-right"] = "auto"
			}
		}
	}

	declarations := cssDeclarationsFromMap(cssMap)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) tableColumnClass(node map[string]interface{}) string {
	style := effectiveStyle(nil, node)
	declarations := cssDeclarationsFromMap(processContentProperties(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	return styleStringFromDeclarations("class", nil, declarations)
}

// fittedContainerClass generates the style class for the outer wrapper of a fitted container.
// It handles -kfx-box-align by converting it to text-align on the outer wrapper (not margin-auto),
// matching Python yj_to_epub_content.py:1375-1381 where fitted (inline-block) elements get
// a wrapper with text-align from box-align so the inline-block is horizontally positioned.
func (r *storylineRenderer) fittedContainerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	if len(style) == 0 {
		return ""
	}
	cssMap := processContentProperties(style, r.resolveResource)

	// Handle -kfx-box-align → text-align conversion for fitted containers.
	// Python yj_to_epub_content.py:1375-1381:
	//   if "-kfx-box-align" in content_style:
	//       container_elem, container_style = self.create_container(
	//           content_elem, content_style, "div", BLOCK_ALIGNED_CONTAINER_PROPERTIES)
	//       container_style["text-align"] = container_style.pop("-kfx-box-align")
	// The outer wrapper gets text-align, which positions the inline-block inner element.
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		delete(cssMap, "-kfx-box-align")
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" {
			cssMap["text-align"] = boxAlign
		}
	}

	declarations := cssDeclarationsFromMap(cssMap)
	if mapFontStyle(style["$12"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, r.nodeLayoutHints(node), declarations)
}

func (r *storylineRenderer) structuredContainerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	declarations := cssDeclarationsFromMap(processContentProperties(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

// tableCellCombineNestedDivs returns true when Python's COMBINE_NESTED_DIVS
// (yj_to_epub_content.py:1408-1448) would merge the cell $269 with its single child $269.
// This happens when the parent and child CSS properties don't overlap (excluding -kfx-style-name).
func (r *storylineRenderer) tableCellCombineNestedDivs(node map[string]interface{}) bool {
	children, ok := asSlice(node["$146"])
	if !ok || len(children) != 1 {
		return false
	}
	child, ok := asMap(children[0])
	if !ok {
		return false
	}
	childContentType, _ := asString(child["$159"])
	if childContentType != "$269" {
		return false
	}
	if _, has145 := asMap(child["$145"]); !has145 {
		return false // child doesn't have text content to merge
	}
	styleID, _ := asString(node["$157"])
	childStyleID, _ := asString(child["$157"])
	if styleID == "" || childStyleID == "" {
		return true // no style conflict
	}
	ownStyle := effectiveStyle(r.styleFragments[styleID], node)
	parentCSS := processContentProperties(ownStyle, r.resolveResource)
	childStyle := effectiveStyle(r.styleFragments[childStyleID], child)
	childCSS := processContentProperties(childStyle, r.resolveResource)
	for prop := range parentCSS {
		if prop == "-kfx-style-name" {
			continue
		}
		if _, exists := childCSS[prop]; exists {
			return false // overlap → no merge
		}
	}
	return true // no overlap → merge
}

func (r *storylineRenderer) tableCellClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))

	// Replicate Python's COMBINE_NESTED_DIVS for table cells.
	// In Python, process_content creates nested divs for cell + content, then
	// COMBINE_NESTED_DIVS merges them when parent has no text, single child div,
	// block display, static position, no float, no overlapping properties.
	// content_style.update(child_sty, replace=False) adds child-only properties.
	// We check overlap against the parent's own CSS properties (before inference),
	// since inferred properties are reverse-inherited from children.
	if children, ok := asSlice(node["$146"]); ok && len(children) == 1 {
		if child, ok := asMap(children[0]); ok {
			childStyleID, _ := asString(child["$157"])
			if childStyleID != "" {
				ownStyle := effectiveStyle(r.styleFragments[styleID], node)
				parentCSS := processContentProperties(ownStyle, r.resolveResource)
				childStyle := effectiveStyle(r.styleFragments[childStyleID], child)
				childCSS := processContentProperties(childStyle, r.resolveResource)
				// Python excludes -kfx-style-name from overlap check
				hasOverlap := false
				for prop := range parentCSS {
					if prop == "-kfx-style-name" {
						continue
					}
					if _, exists := childCSS[prop]; exists {
						hasOverlap = true
						break
					}
				}
				if !hasOverlap {
					style = mergeStyleValues(style, childStyle)
				}
			}
		}
	}

	cssMap := processContentProperties(style, r.resolveResource)

	// Handle -kfx-box-align → text-align conversion for table cells.
	// In Python's process_content (yj_to_epub_content.py), -kfx-box-align is popped from
	// td element styles. Since cssDeclarationsFromMap strips -kfx- prefixed properties,
	// we convert it to text-align here to preserve alignment information, matching the old
	// tableCellStyleDeclarations which used mapBoxAlign to convert $580/$34 values to text-align.
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" || boxAlign == "justify" {
			if _, exists := cssMap["text-align"]; !exists {
				cssMap["text-align"] = boxAlign
			}
		}
		delete(cssMap, "-kfx-box-align")
	}

	// Strip -kfx-attrib-colspan/rowspan from style — already handled as direct HTML attributes.
	// cssDeclarationsFromMap now preserves -kfx-attrib-* properties, but for table cells
	// these are redundant (extracted from $148/$149 into colspan/rowspan attrs directly).
	delete(cssMap, "-kfx-attrib-colspan")
	delete(cssMap, "-kfx-attrib-rowspan")

	declarations := cssDeclarationsFromMap(cssMap)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) inlineContainerClass(styleID string, node map[string]interface{}) string {
	style := effectiveStyle(r.styleFragments[styleID], node)
	declarations := cssDeclarationsFromMap(processContentProperties(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

var blockAlignedContainerProperties = map[string]bool{
	"-kfx-attrib-colspan": true, "-kfx-attrib-rowspan": true,
	"-kfx-box-align": true, "-kfx-heading-level": true, "-kfx-layout-hints": true,
	"-kfx-table-vertical-align": true,
	"box-sizing": true,
	"float": true,
	"margin-left": true, "margin-right": true, "margin-top": true, "margin-bottom": true,
	"overflow": true,
	"page-break-after": true, "page-break-before": true, "page-break-inside": true,
	"text-indent": true,
	"transform": true, "transform-origin": true,
}

var reverseHeritablePropertiesExcludes = map[string]bool{
	"-amzn-page-align":                   true,
	"-kfx-user-margin-bottom-percentage": true,
	"-kfx-user-margin-left-percentage":   true,
	"-kfx-user-margin-right-percentage":  true,
	"-kfx-user-margin-top-percentage":    true,
	"font-size":  true,
	"line-height": true,
}

func isBlockContainerProperty(prop string) bool {
	if blockAlignedContainerProperties[prop] || prop == "display" {
		return true
	}
	return heritableProperties[prop] && !reverseHeritablePropertiesExcludes[prop]
}

func (r *storylineRenderer) imageClasses(node map[string]interface{}) (string, string) {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = r.adjustRenderableStyle(style, node)
	if len(style) == 0 {
		return "", ""
	}
	cssMap := processContentProperties(style, r.resolveResource)
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}

	// Handle -kfx-box-align → text-align conversion.
	// The old imageWrapperStyleDeclarations used mapBoxAlign to convert $580/$34
	// directly to text-align. processContentProperties outputs -kfx-box-align instead.
	// Convert it to text-align for CSS output, matching the old behavior.
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" || boxAlign == "justify" {
			cssMap["text-align"] = boxAlign
		}
		delete(cssMap, "-kfx-box-align")
	}

	// Partition properties between wrapper div and image element, matching Python's
	// create_container(BLOCK_CONTAINER_PROPERTIES) in yj_to_epub_content.py:1324.
	// Container gets: REVERSE_HERITABLE_PROPERTIES | BLOCK_ALIGNED_CONTAINER_PROPERTIES | {"display"}.
	// Image keeps everything else. No properties are dropped.
	wrapperProps := map[string]string{}
	imageProps := map[string]string{}
	for prop, val := range cssMap {
		if isBlockContainerProperty(prop) {
			wrapperProps[prop] = val
		} else {
			imageProps[prop] = val
		}
	}

	// Python yj_to_epub_content.py:1328-1331: when container has float, move % width
	// from inner image to container and set image width to 100%.
	if _, hasFloat := wrapperProps["float"]; hasFloat {
		if w, ok := imageProps["width"]; ok && strings.HasSuffix(w, "%") {
			wrapperProps["width"] = w
			imageProps["width"] = "100%"
		}
	}

	wrapperDecls := cssDeclarationsFromMap(wrapperProps)
	imageDecls := cssDeclarationsFromMap(imageProps)

	switch {
	case len(wrapperDecls) > 0 && len(imageDecls) > 0:
		return styleStringFromDeclarations(baseName, nil, wrapperDecls), styleStringFromDeclarations(baseName, nil, imageDecls)
	case len(wrapperDecls) > 0:
		return styleStringFromDeclarations(baseName, nil, wrapperDecls), ""
	case len(imageDecls) > 0:
		return "", styleStringFromDeclarations(baseName, nil, imageDecls)
	default:
		return "", ""
	}
}

func (r *storylineRenderer) adjustRenderableStyle(style map[string]interface{}, node map[string]interface{}) map[string]interface{} {
	if len(style) == 0 {
		return style
	}
	if fitTight, _ := asBool(node["$784"]); fitTight {
		if value := cssLengthProperty(style["$56"], "$56"); value == "100%" {
			style = cloneMap(style)
			delete(style, "$56")
		}
	}
	return style
}

func (r *storylineRenderer) headingClass(styleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	className := r.headingClassName(styleID, style)
	declarations := cssDeclarationsFromMap(processContentProperties(style, r.resolveResource))
	if mapFontStyle(style["$12"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if style["$36"] == nil && activeTextIndentNeedsReset(r.activeBodyDefaults) {
		declarations = append(declarations, "text-indent: 0")
	}
	if len(declarations) == 0 {
		return ""
	}
	return styleStringFromDeclarations(className, []string{"heading"}, declarations)
}

func (r *storylineRenderer) paragraphClass(styleID string, annotationStyleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	// Merge link style inheritance: when paragraph doesn't have certain properties,
	// inherit them from the link (annotation) style. This preserves the behavior of
	// the old paragraphStyleDeclarations link inheritance block (kfx.go:1164-1201).
	linkStyle := effectiveStyle(r.styleFragments[annotationStyleID], nil)
	if linkStyle != nil {
		// Merge link color properties ($576=visited-color, $577=link-color) for color resolution
		for _, yjProp := range []string{"$576", "$577"} {
			if _, ok := style[yjProp]; !ok {
				if val, ok := linkStyle[yjProp]; ok {
					style[yjProp] = val
				}
			}
		}
		// Merge link font/typographic properties when paragraph doesn't have them
		for _, yjProp := range []string{"$11", "$12", "$13", "$583", "$41"} {
			if _, ok := style[yjProp]; !ok {
				if val, ok := linkStyle[yjProp]; ok {
					style[yjProp] = val
				}
			}
		}
	}
	cssMap := processContentProperties(style, r.resolveResource)
	// Resolve link color: if no explicit color but -kfx-link-color == -kfx-visited-color,
	// set color to that value. This preserves what colorDeclarations(style, linkStyle) did
	// in the old paragraphStyleDeclarations, and matches simplifyStylesElementFull's <a> tag logic.
	if _, hasColor := cssMap["color"]; !hasColor {
		linkColor, hasLink := cssMap["-kfx-link-color"]
		visitedColor, hasVisited := cssMap["-kfx-visited-color"]
		if hasLink && hasVisited && linkColor == visitedColor {
			cssMap["color"] = linkColor
		}
	}
	declarations := cssDeclarationsFromMap(cssMap)
	if mapFontStyle(style["$12"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if style["$36"] == nil && activeTextIndentNeedsReset(r.activeBodyDefaults) {
		declarations = append(declarations, "text-indent: 0")
	}
	if os.Getenv("KFX_DEBUG_PARAGRAPH_STYLE") != "" {
		fmt.Fprintf(os.Stderr, "paragraph style=%s body=%s decls=%v\n", styleID, r.activeBodyClass, declarations)
	}
	className := ""
	if len(declarations) > 0 {
		baseName := "class"
		if styleID != "" {
			baseName = r.styleBaseName(styleID)
		}
		className = styleStringFromDeclarations(baseName, nil, declarations)
	}
	if annotationStyleID != "" {
		_ = r.linkClass(annotationStyleID, true)
	}
	return className
}

func (r *storylineRenderer) linkClass(styleID string, suppressColor bool) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	cssMap := processContentProperties(style, r.resolveResource)
	// Always resolve link color: if no explicit color but -kfx-link-color == -kfx-visited-color,
	// set color to that value. This matches simplifyStylesElementFull's <a> tag logic.
	// Previously we suppressed color when suppressColor was true (for paragraphs that
	// handle color via link style inheritance). But simplify_styles will strip the
	// redundant color from <a> if it matches inherited, so we don't need to suppress.
	if _, hasColor := cssMap["color"]; !hasColor {
		linkColor, hasLink := cssMap["-kfx-link-color"]
		visitedColor, hasVisited := cssMap["-kfx-visited-color"]
		if hasLink && hasVisited && linkColor == visitedColor {
			cssMap["color"] = linkColor
		}
	}
	// Always strip -kfx- properties (they're not real CSS and will appear in the
	// style catalog if not removed here).
	delete(cssMap, "-kfx-link-color")
	delete(cssMap, "-kfx-visited-color")
	declarations := cssDeclarationsFromMap(cssMap)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) spanClass(styleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	declarations := cssDeclarationsFromMap(processContentProperties(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) resolveText(ref map[string]interface{}) string {
	return resolveContentText(r.contentFragments, ref)
}

func hasRenderableContainer(node map[string]interface{}) bool {
	_, hasStyle := asString(node["$157"])
	children, hasChildren := asSlice(node["$146"])
	_, hasImage := asString(node["$175"])
	_, hasText := asMap(node["$145"])
	return hasStyle && !hasImage && !hasText && (!hasChildren || len(children) == 0)
}

func promotedBodyContainer(nodes []interface{}) (string, []interface{}, bool) {
	if len(nodes) != 1 {
		return "", nil, false
	}
	node, ok := asMap(nodes[0])
	if !ok {
		return "", nil, false
	}
	styleID, _ := asString(node["$157"])
	children, ok := asSlice(node["$146"])
	if !ok || len(children) == 0 || styleID == "" {
		return "", nil, false
	}
	if _, ok := asMap(node["$145"]); ok {
		return "", nil, false
	}
	if _, ok := asString(node["$175"]); ok {
		return "", nil, false
	}
	if headingLevel(node) > 0 {
		return "", nil, false
	}
	return styleID, children, true
}

func defaultInheritedBodyStyle() map[string]interface{} {
	zero := 0.0
	return map[string]interface{}{
		"$11": "default,serif",
		"$12": "$350",
		"$13": "$350",
		"$36": map[string]interface{}{
			"$306": "$308",
			"$307": &zero,
		},
	}
}

func (r *storylineRenderer) inferBodyStyleValues(nodes []interface{}, parentStyle map[string]interface{}) map[string]interface{} {
	return r.inferSharedHeritableStyle(parentStyle, nodes)
}

func (r *storylineRenderer) inferPromotedBodyStyle(nodes []interface{}) map[string]interface{} {
	return r.inferSharedHeritableStyle(nil, nodes)
}

func (r *storylineRenderer) inferPromotedStyleValues(node map[string]interface{}) map[string]interface{} {
	children, ok := asSlice(node["$146"])
	if !ok || len(children) == 0 {
		return nil
	}
	styleID, _ := asString(node["$157"])
	return r.inferSharedHeritableStyle(effectiveStyle(r.styleFragments[styleID], node), children)
}

func (r *storylineRenderer) inferSharedHeritableStyle(parentStyle map[string]interface{}, nodes []interface{}) map[string]interface{} {
	if len(nodes) == 0 {
		return nil
	}
	type valueCount struct {
		count int
		raw   interface{}
	}
	const reverseInheritanceFraction = 0.8
	keys := []string{"$11", "$12", "$13", "$34", "$36", "$41", "$583"}
	valueCounts := map[string]map[string]*valueCount{}
	numChildren := 0
	debugInfer := os.Getenv("KFX_DEBUG_INFER_COUNTS") != ""
	debugStyleIDs := make([]string, 0, len(nodes))
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		styleID, _ := asString(node["$157"])
		if debugInfer {
			debugStyleIDs = append(debugStyleIDs, styleID)
		}
		style := effectiveStyle(r.styleFragments[styleID], node)
		if childPromoted := r.inferPromotedStyleValues(node); len(childPromoted) > 0 {
			style = mergeStyleValues(style, childPromoted)
		}
		numChildren++
		if len(style) == 0 {
			continue
		}
		for _, key := range keys {
			rawValue, ok := style[key]
			if !ok {
				continue
			}
			valueKey := fmt.Sprintf("%#v", rawValue)
			if valueCounts[key] == nil {
				valueCounts[key] = map[string]*valueCount{}
			}
			entry := valueCounts[key][valueKey]
			if entry == nil {
				entry = &valueCount{raw: rawValue}
				valueCounts[key][valueKey] = entry
			}
			entry.count++
		}
	}
	if numChildren == 0 {
		return nil
	}
	values := map[string]interface{}{}
	for _, key := range keys {
		counts := valueCounts[key]
		if len(counts) == 0 {
			continue
		}
		var (
			bestKey   string
			bestValue interface{}
			bestCount int
			total     int
		)
		for valueKey, entry := range counts {
			total += entry.count
			if entry.count > bestCount {
				bestKey = valueKey
				bestValue = entry.raw
				bestCount = entry.count
			}
		}
		if bestKey == "" {
			continue
		}
		if total < numChildren && (parentStyle == nil || parentStyle[key] == nil) {
			continue
		}
		if float64(bestCount) >= float64(numChildren)*reverseInheritanceFraction {
			values[key] = bestValue
		}
	}
	if len(values) == 0 {
		if debugInfer {
			fmt.Fprintf(os.Stderr, "infer none numChildren=%d styles=%v counts=", numChildren, debugStyleIDs)
			for _, key := range keys {
				if len(valueCounts[key]) == 0 {
					continue
				}
				fmt.Fprintf(os.Stderr, " %s:{", key)
				first := true
				for valueKey, entry := range valueCounts[key] {
					if !first {
						fmt.Fprint(os.Stderr, ", ")
					}
					first = false
					fmt.Fprintf(os.Stderr, "%s=%d", valueKey, entry.count)
				}
				fmt.Fprint(os.Stderr, "}")
			}
			fmt.Fprintln(os.Stderr)
		}
		return nil
	}
	if debugInfer {
		fmt.Fprintf(os.Stderr, "infer values numChildren=%d styles=%v values=%#v\n", numChildren, debugStyleIDs, values)
	}
	return values
}

func headingLevel(node map[string]interface{}) int {
	value, ok := node["$790"]
	if !ok {
		return 0
	}
	level, _ := asInt(value)
	return level
}

func fullParagraphAnnotationStyleID(node map[string]interface{}, text string) string {
	annotations, ok := asSlice(node["$142"])
	if !ok || len(annotations) == 0 {
		return ""
	}
	runeCount := len([]rune(text))
	for _, raw := range annotations {
		annotationMap, ok := asMap(raw)
		if !ok || !annotationCoversWholeText(annotationMap, runeCount) {
			continue
		}
		styleID, _ := asString(annotationMap["$157"])
		return styleID
	}
	return ""
}

func annotationCoversWholeText(annotationMap map[string]interface{}, runeCount int) bool {
	if annotationMap == nil || runeCount == 0 {
		return false
	}
	start, hasStart := asInt(annotationMap["$143"])
	length, hasLength := asInt(annotationMap["$144"])
	_, hasAnchor := asString(annotationMap["$179"])
	return hasAnchor && hasStart && hasLength && start == 0 && length >= runeCount
}

func (r *storylineRenderer) headingClassName(styleID string, style map[string]interface{}) string {
	simplified := uniquePartOfLocalSymbol(styleID, r.symFmt)
	if simplified != "" {
		return "heading_" + simplified
	}
	return "heading_" + styleID
}

func filterBodyDefaultDeclarations(declarations []string, bodyDefaults map[string]bool) []string {
	if len(declarations) == 0 {
		return declarations
	}
	filtered := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		if bodyDefaults != nil && bodyDefaults[declaration] {
			continue
		}
		filtered = append(filtered, declaration)
	}
	return filtered
}

func activeTextIndentNeedsReset(bodyDefaults map[string]bool) bool {
	if len(bodyDefaults) == 0 {
		return false
	}
	for declaration := range bodyDefaults {
		if strings.HasPrefix(declaration, "text-indent: ") {
			return declaration != "text-indent: 0"
		}
	}
	return false
}

func bodyDefaultsInclude(bodyDefaults map[string]bool, declaration string) bool {
	if len(bodyDefaults) == 0 {
		return false
	}
	return bodyDefaults[declaration]
}

func (r *storylineRenderer) applyAnnotations(text string, node map[string]interface{}) []htmlPart {
	annotations, ok := asSlice(node["$142"])
	type event struct {
		start int
		end   int
		open  func(parent *htmlElement) *htmlElement
		close func(opened *htmlElement)
	}
	type activeEvent struct {
		event  event
		opened *htmlElement
	}
	runes := []rune(text)
	if dropcapLines, hasDropcapLines := asInt(node["$125"]); hasDropcapLines && dropcapLines > 0 {
		if dropcapChars, hasDropcapChars := asInt(node["$126"]); hasDropcapChars && dropcapChars > 0 {
			dropcap := map[string]interface{}{
				"$143": 0,
				"$144": dropcapChars,
				"$125": dropcapLines,
			}
			annotations = append([]interface{}{dropcap}, annotations...)
			ok = true
		}
	}
	events := make([]event, 0, len(annotations))
	if ok {
		for _, raw := range annotations {
			annotationMap, ok := asMap(raw)
			if !ok {
				continue
			}
			start, hasStart := asInt(annotationMap["$143"])
			length, hasLength := asInt(annotationMap["$144"])
			if !hasStart || !hasLength || length <= 0 || start < 0 || start >= len(runes) {
				continue
			}
			end := start + length
			if end > len(runes) {
				end = len(runes)
			}
			anchorID, _ := asString(annotationMap["$179"])
			styleID, _ := asString(annotationMap["$157"])
			dropcapClass := ""
			if lines, ok := asInt(annotationMap["$125"]); ok && lines > 0 {
				dropcapClass = r.dropcapClass(lines)
			}
			if debugAnchors := os.Getenv("KFX_DEBUG_ANCHORS"); debugAnchors != "" && anchorID != "" {
				for _, wanted := range strings.Split(debugAnchors, ",") {
					if strings.TrimSpace(wanted) == anchorID {
						fmt.Fprintf(os.Stderr, "annotation anchor=%s style=%s value=%#v\n", anchorID, styleID, annotationMap)
					}
				}
			}
			href := r.anchorHref(anchorID)
			rubyName, hasRubyName := asString(annotationMap["$757"])
			if hasRubyName && rubyName != "" {
				rubyIDs := r.rubyAnnotationIDs(annotationMap, end-start)
				var rubyElement *htmlElement
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						rubyElement = &htmlElement{Tag: "ruby", Attrs: map[string]string{}}
						parent.Children = append(parent.Children, rubyElement)
						rb := &htmlElement{Tag: "rb", Attrs: map[string]string{}}
						rubyElement.Children = append(rubyElement.Children, rb)
						return rb
					},
					close: func(opened *htmlElement) {
						if opened == nil || rubyElement == nil {
							return
						}
						for _, rubyID := range rubyIDs {
							rt := &htmlElement{Tag: "rt", Attrs: map[string]string{}, Children: r.rubyContentParts(rubyName, rubyID)}
							rubyElement.Children = append(rubyElement.Children, rt)
						}
					},
				})
				continue
			}
			if href != "" {
				styleAttr := mergeStyleStrings(
					r.linkClass(styleID, annotationCoversWholeText(annotationMap, len(runes))),
					dropcapClass,
				)
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						attrs := map[string]string{"href": href}
						if styleAttr != "" {
							attrs["style"] = styleAttr
						}
						element := &htmlElement{Tag: "a", Attrs: attrs}
						parent.Children = append(parent.Children, element)
						return element
					},
				})
				continue
			}
			if styleAttr := mergeStyleStrings(r.spanClass(styleID), dropcapClass); styleAttr != "" {
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						element := &htmlElement{Tag: "span", Attrs: map[string]string{"style": styleAttr}}
						parent.Children = append(parent.Children, element)
						return element
					},
				})
			}
		}
	}
	if len(events) == 0 {
		return splitTextHTMLParts(text)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].start == events[j].start {
			return events[i].end > events[j].end
		}
		return events[i].start < events[j].start
	})
	root := &htmlElement{Attrs: map[string]string{}}
	stack := []*activeEvent{{opened: root}}
	last := 0
	for index, rch := range runes {
		if last < index {
			appendTextHTMLParts(stack[len(stack)-1].opened, string(runes[last:index]))
			last = index
		}
		for _, ev := range events {
			if ev.start == index {
				opened := ev.open(stack[len(stack)-1].opened)
				stack = append(stack, &activeEvent{event: ev, opened: opened})
			}
		}
		appendTextHTMLParts(stack[len(stack)-1].opened, string(rch))
		last = index + 1
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].end == index+1 {
				if len(stack) > 1 {
					active := stack[len(stack)-1]
					if active.event.close != nil {
						active.event.close(active.opened)
					}
					stack = stack[:len(stack)-1]
				}
			}
		}
	}
	if last < len(runes) {
		appendTextHTMLParts(stack[len(stack)-1].opened, string(runes[last:]))
	}

	// Port of Python add_content (yj_to_epub_content.py:364-370):
	// Python wraps ALL text in <span> elements via SubElement(parent, "span").
	// This ensures no bare text nodes exist in the tree, which is critical for
	// simplify_styles reverse inheritance (Python skips when elem.text or elem.tail
	// is set, line 1875-1876). During simplify_styles, empty spans are unwrapped
	// (matching etree.strip_tags in epub_output.py:783-789).
	for i, child := range root.Children {
		switch child.(type) {
		case htmlText, *htmlText:
			root.Children[i] = &htmlElement{
				Tag:      "span",
				Attrs:    map[string]string{},
				Children: []htmlPart{child},
			}
		}
	}

	return root.Children
}

func (r *storylineRenderer) rubyAnnotationIDs(annotationMap map[string]interface{}, eventLength int) []int {
	if annotationMap == nil {
		return nil
	}
	if rubyID, ok := asInt(annotationMap["$758"]); ok {
		return []int{rubyID}
	}
	rawIDs, ok := asSlice(annotationMap["$759"])
	if !ok {
		return nil
	}
	ids := make([]int, 0, len(rawIDs))
	for _, raw := range rawIDs {
		entry, ok := asMap(raw)
		if !ok {
			continue
		}
		if rubyID, ok := asInt(entry["$758"]); ok {
			ids = append(ids, rubyID)
		}
	}
	return ids
}

func (r *storylineRenderer) rubyContentParts(rubyName string, rubyID int) []htmlPart {
	content := r.getRubyContent(rubyName, rubyID)
	if content == nil {
		return nil
	}
	if ref, ok := asMap(content["$145"]); ok {
		if text := r.resolveText(ref); text != "" {
			return splitTextHTMLParts(text)
		}
	}
	if children, ok := asSlice(content["$146"]); ok {
		parts := make([]htmlPart, 0, len(children))
		for _, child := range children {
			if rendered := r.renderInlinePart(child, 0); rendered != nil {
				parts = append(parts, rendered)
			}
		}
		return parts
	}
	return nil
}

func (r *storylineRenderer) getRubyContent(rubyName string, rubyID int) map[string]interface{} {
	group := r.rubyGroups[rubyName]
	if group == nil {
		return nil
	}
	children, _ := asSlice(group["$146"])
	for _, raw := range children {
		switch typed := raw.(type) {
		case string:
			if content := r.rubyContents[typed]; content != nil {
				if id, ok := asInt(content["$758"]); ok && id == rubyID {
					return cloneMap(content)
				}
			}
		default:
			entry, ok := asMap(raw)
			if !ok {
				continue
			}
			if id, ok := asInt(entry["$758"]); ok && id == rubyID {
				return cloneMap(entry)
			}
		}
	}
	return nil
}

func (r *storylineRenderer) dropcapClass(lines int) string {
	if lines <= 0 {
		return ""
	}
	return styleStringFromDeclarations("class", nil, []string{
		"float: left",
		fmt.Sprintf("font-size: %dem", lines),
		"line-height: 100%",
		"margin-bottom: 0",
		"margin-right: 0.1em",
		"margin-top: 0",
	})
}

func (r *storylineRenderer) anchorIDForPosition(positionID int, offset int) string {
	offsets := r.positionAnchorID[positionID]
	if offsets == nil {
		return ""
	}
	return offsets[offset]
}

func (r *storylineRenderer) anchorOnlyMovable(positionID int, offset int) bool {
	offsets := r.positionAnchors[positionID]
	if offsets == nil {
		return false
	}
	names := offsets[offset]
	if len(names) == 0 {
		return false
	}
	for _, name := range names {
		if strings.HasPrefix(name, "$798_") {
			return false
		}
	}
	return true
}

func (r *storylineRenderer) applyPositionAnchors(element *htmlElement, positionID int, isFirstVisible bool) {
	if element == nil || positionID == 0 {
		return
	}
	if os.Getenv("KFX_DEBUG") != "" && (positionID == 1110 || positionID == 1111 || positionID == 1177 || positionID == 1178) {
		fmt.Fprintf(os.Stderr, "apply anchors pos=%d tag=%s first=%v raw=%v ids=%v\n", positionID, element.Tag, isFirstVisible, r.positionAnchors[positionID], r.positionAnchorID[positionID])
	}
	offsets := r.positionAnchors[positionID]
	if len(offsets) == 0 {
		return
	}
	if anchorID := r.anchorIDForPosition(positionID, 0); anchorID != "" {
		if !isFirstVisible && !strings.HasPrefix(anchorID, "id__212_") {
			element.Attrs["id"] = anchorID
			r.emittedAnchorIDs[anchorID] = true
			r.registerAnchorElementNames(positionID, 0, anchorID)
			if os.Getenv("KFX_DEBUG") != "" && (positionID == 1110 || positionID == 1111 || positionID == 1177 || positionID == 1178 || positionID == 1007 || positionID == 1053) {
				fmt.Fprintf(os.Stderr, "set id pos=%d tag=%s id=%s class=%s\n", positionID, element.Tag, anchorID, element.Attrs["class"])
			}
		}
	}
	ordered := make([]int, 0, len(offsets))
	for offset := range offsets {
		if offset > 0 {
			ordered = append(ordered, offset)
		}
	}
	sort.Ints(ordered)
	for _, offset := range ordered {
		anchorID := r.anchorIDForPosition(positionID, offset)
		if anchorID == "" {
			continue
		}
		target := locateOffset(element, offset)
		if target == nil {
			continue
		}
		if target.Attrs == nil {
			target.Attrs = map[string]string{}
		}
		if target.Attrs["id"] == "" {
			target.Attrs["id"] = anchorID
			r.emittedAnchorIDs[anchorID] = true
			r.registerAnchorElementNames(positionID, offset, anchorID)
		}
	}
}

func (r *storylineRenderer) registerAnchorElementNames(positionID int, offset int, anchorID string) {
	if r == nil || anchorID == "" {
		return
	}
	offsets := r.positionAnchors[positionID]
	if offsets == nil {
		return
	}
	names := offsets[offset]
	if len(names) == 0 {
		return
	}
	if r.anchorNamesByID == nil {
		r.anchorNamesByID = map[string][]string{}
	}
	seen := map[string]bool{}
	for _, existing := range r.anchorNamesByID[anchorID] {
		seen[existing] = true
	}
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		r.anchorNamesByID[anchorID] = append(r.anchorNamesByID[anchorID], name)
	}
}

func (r *storylineRenderer) headingLevelForPosition(positionID int, offset int) int {
	if r == nil || positionID == 0 || r.anchorHeadingLevel == nil {
		return 0
	}
	offsets := r.positionAnchors[positionID]
	if offsets == nil {
		return 0
	}
	for _, name := range offsets[offset] {
		if level := r.anchorHeadingLevel[name]; level > 0 {
			return level
		}
	}
	return 0
}

func (r *storylineRenderer) consumeVisibleElement() bool {
	isFirst := !r.firstVisibleSeen
	r.firstVisibleSeen = true
	return isFirst
}

