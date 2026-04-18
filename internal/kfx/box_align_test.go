package kfx

import (
	"sort"
	"strings"
	"testing"
)

// rendererForBoxAlignTests creates a storylineRenderer with synthetic style fragments
// that exercise the Python box-align handling paths from yj_to_epub_content.py.
func rendererForBoxAlignTests() storylineRenderer {
	return storylineRenderer{
		contentFragments: map[string][]string{
			"content": {"Hello"},
		},
		resourceHrefByID: map[string]string{},
		anchorToFilename: map[string]string{},
		positionToSection: map[int]string{
			1: "cX",
		},
		positionAnchors:  map[int]map[int][]string{},
		positionAnchorID: map[int]map[int]string{},
		emittedAnchorIDs: map[string]bool{},
		styles:           newStyleCatalog(),
		// Style fragments matching Python's $157 style data.
		// Each key is a style ID (e.g. "sImgCenter") and the value is
		// the YJ property map that would come from the KFX data.
		styleFragments: map[string]map[string]interface{}{
			// Image with box-align:center + width:50% — exercises Python path 1:
			// yj_to_epub_content.py:1324 create_container(BLOCK_CONTAINER_PROPERTIES)
			// then line 1335: box-align → text-align on wrapper
			"sImgCenter": {
				"$580": "$320", // -kfx-box-align: center
				"$56":  map[string]interface{}{"$307": 50.0, "$306": "$314"}, // width: 50%
			},
			// Image with box-align:right + width:30% — exercises text-align:right
			"sImgRight": {
				"$580": "$61", // -kfx-box-align: right
				"$56":  map[string]interface{}{"$307": 30.0, "$306": "$314"}, // width: 30%
			},
			// Image with box-align:center + width + float:left
			// Exercises yj_to_epub_content.py:1328-1331:
			// float present → move % width from image to wrapper
			"sImgFloat": {
				"$580": "$320", // -kfx-box-align: center
				"$56":  map[string]interface{}{"$307": 40.0, "$306": "$314"}, // width: 40%
				"$140": "$59", // float: left
			},
			// Container div with box-align:center + width:100%
			// Exercises Python path 3: yj_to_epub_content.py:1390-1404
			// box-align → margin-left/right auto (when element has width)
			"sDivCenter": {
				"$580": "$320", // -kfx-box-align: center
				"$56":  map[string]interface{}{"$307": 100.0, "$306": "$314"}, // width: 100%
			},
			// Container div with box-align:center + no width
			// Python: box-align is popped but no margin-auto (no width condition)
			"sDivCenterNoWidth": {
				"$580": "$320", // -kfx-box-align: center
			},
			// Container div with box-align:right + width
			"sDivRight": {
				"$580": "$61", // -kfx-box-align: right
				"$56":  map[string]interface{}{"$307": 70.0, "$306": "$314"}, // width: 70%
			},
			// Image with only width (no box-align) — baseline image partition
			"sImgPlain": {
				"$56": map[string]interface{}{"$307": 25.0, "$306": "$314"}, // width: 25%
			},
		},
	}
}

// parseCSSDeclarations extracts property:value pairs from a CSS declaration string.
func parseCSSDeclarations(style string) map[string]string {
	result := map[string]string{}
	for _, decl := range strings.Split(style, ";") {
		decl = strings.TrimSpace(decl)
		if idx := strings.Index(decl, ":"); idx >= 0 {
			prop := strings.TrimSpace(decl[:idx])
			val := strings.TrimSpace(decl[idx+1:])
			if prop != "" && !strings.HasPrefix(prop, "-kfx-") {
				result[prop] = val
			}
		}
	}
	return result
}

// cssPropertiesSorted returns sorted CSS properties from a style string, for comparison.
func cssPropertiesSorted(style string) []string {
	props := parseCSSDeclarations(style)
	result := make([]string, 0, len(props))
	for prop, val := range props {
		result = append(result, prop+": "+val)
	}
	sort.Strings(result)
	return result
}

// TestImageClassesBoxAlign converts -kfx-box-align to text-align on wrapper, matching
// Python yj_to_epub_content.py:1335:
//   container_style["text-align"] = container_style.pop("-kfx-box-align")
//
// The wrapper div should get text-align, the image should get width.
func TestImageClassesBoxAlign(t *testing.T) {
	r := rendererForBoxAlignTests()

	tests := []struct {
		name              string
		styleID           string
		wantWrapperProps  []string // sorted "prop: val" expected on wrapper
		wantImageProps    []string // sorted "prop: val" expected on image
	}{
		{
			name:     "center aligned image",
			styleID:  "sImgCenter",
			wantWrapperProps: []string{
				"text-align: center",
			},
			wantImageProps: []string{
				"width: 50%",
			},
		},
		{
			name:     "right aligned image",
			styleID:  "sImgRight",
			wantWrapperProps: []string{
				"text-align: right",
			},
			wantImageProps: []string{
				"width: 30%",
			},
		},
		{
			name:     "plain image no box-align",
			styleID:  "sImgPlain",
			wantWrapperProps: nil, // no wrapper needed — no block container props
			wantImageProps: []string{
				"width: 25%",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := map[string]interface{}{"$157": tt.styleID}
			wrapperClass, imageClass := r.imageClasses(node)

			if len(tt.wantWrapperProps) == 0 {
				if wrapperClass != "" {
					// Check that wrapper has no real CSS (may have -kfx- metadata only)
					props := parseCSSDeclarations(wrapperClass)
					if len(props) > 0 {
						t.Errorf("expected no wrapper CSS properties, got: %v", props)
					}
				}
			} else {
				if wrapperClass == "" {
					t.Fatalf("expected wrapper class, got empty")
				}
				gotProps := cssPropertiesSorted(wrapperClass)
				if len(gotProps) != len(tt.wantWrapperProps) {
					t.Errorf("wrapper props = %v, want %v", gotProps, tt.wantWrapperProps)
				}
				for i := range gotProps {
					if i < len(tt.wantWrapperProps) && gotProps[i] != tt.wantWrapperProps[i] {
						t.Errorf("wrapper prop[%d] = %q, want %q", i, gotProps[i], tt.wantWrapperProps[i])
					}
				}
			}

			if imageClass == "" {
				t.Fatalf("expected image class, got empty")
			}
			gotImgProps := cssPropertiesSorted(imageClass)
			if len(gotImgProps) != len(tt.wantImageProps) {
				t.Errorf("image props = %v, want %v", gotImgProps, tt.wantImageProps)
			}
			for i := range gotImgProps {
				if i < len(tt.wantImageProps) && gotImgProps[i] != tt.wantImageProps[i] {
					t.Errorf("image prop[%d] = %q, want %q", i, gotImgProps[i], tt.wantImageProps[i])
				}
			}
		})
	}
}

// TestImageClassesFloatWidthTransfer moves % width from image to wrapper when
// the wrapper has float, matching Python yj_to_epub_content.py:1328-1331:
//
//	if "float" in container_style:
//	    if width_or_height in content_style and content_style[width_or_height].endswith("%"):
//	        container_style[width_or_height] = content_style[width_or_height]
//	        content_style[width_or_height] = "100%"
func TestImageClassesFloatWidthTransfer(t *testing.T) {
	r := rendererForBoxAlignTests()

	node := map[string]interface{}{"$157": "sImgFloat"}
	wrapperClass, imageClass := r.imageClasses(node)

	if wrapperClass == "" {
		t.Fatal("expected wrapper class for floated image, got empty")
	}
	wrapperProps := parseCSSDeclarations(wrapperClass)

	// Wrapper should have the % width moved from image
	if wrapperProps["width"] != "40%" {
		t.Errorf("wrapper width = %q, want %q", wrapperProps["width"], "40%")
	}
	// Wrapper should have text-align from box-align conversion
	if wrapperProps["text-align"] != "center" {
		t.Errorf("wrapper text-align = %q, want %q", wrapperProps["text-align"], "center")
	}

	// Image should have width set to 100%
	imageProps := parseCSSDeclarations(imageClass)
	if imageProps["width"] != "100%" {
		t.Errorf("image width = %q, want %q", imageProps["width"], "100%")
	}
}

// TestContainerClassBoxAlign converts -kfx-box-align to margin-left/right auto
// for container elements with width, matching Python yj_to_epub_content.py:1390-1404:
//
//	if "-kfx-box-align" in content_style:
//	    box_align = content_style.pop("-kfx-box-align")
//	    if box_align in ["left", "right", "center"]:
//	        if "width" in content_style or content_elem.tag == "table":
//	            if box_align != "left":  content_style["margin-left"] = "auto"
//	            if box_align != "right": content_style["margin-right"] = "auto"
func TestContainerClassBoxAlign(t *testing.T) {
	r := rendererForBoxAlignTests()

	tests := []struct {
		name        string
		styleID     string
		wantProps   []string // sorted "prop: val" expected (non-default, non-inherited)
	}{
		{
			name:    "center with width → margin-left/right auto",
			styleID: "sDivCenter",
			// box-align center + width → margin-left:auto, margin-right:auto
			// margin-left/right auto are defaults → stripped later by simplify_styles
			// But at the containerClass level they should be present
			wantProps: []string{
				"margin-left: auto",
				"margin-right: auto",
			},
		},
		{
			name:    "right with width → margin-left auto only",
			styleID: "sDivRight",
			wantProps: []string{
				"margin-left: auto",
			},
		},
		{
			name:    "center no width → box-align dropped, no margin auto",
			styleID: "sDivCenterNoWidth",
			// No width → Python doesn't add margin-auto, box-align is popped
			// Result: no CSS properties from box-align
			wantProps: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := map[string]interface{}{"$157": tt.styleID}
			got := r.containerClass(node)

			if len(tt.wantProps) == 0 {
				if got != "" {
					// May have only -kfx- metadata, check for real CSS props
					props := parseCSSDeclarations(got)
					if len(props) > 0 {
						t.Errorf("expected no real CSS properties, got: %v", props)
					}
				}
				return
			}

			if got == "" {
				t.Fatalf("expected container class, got empty")
			}

			props := parseCSSDeclarations(got)
			for _, want := range tt.wantProps {
				idx := strings.Index(want, ":")
				prop := want[:idx]
				val := strings.TrimSpace(want[idx+1:])
				gotVal, ok := props[prop]
				if !ok {
					t.Errorf("missing property %q in %v", prop, props)
				} else if gotVal != val {
					t.Errorf("property %q = %q, want %q", prop, gotVal, val)
				}
			}
		})
	}
}
