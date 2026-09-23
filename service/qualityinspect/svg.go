package qualityinspect

import (
	"encoding/xml"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

var svgOpening = regexp.MustCompile(`<svg(?:\s|/?>)`)
var svgIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
var svgLocalRef = regexp.MustCompile(`^#([A-Za-z_][A-Za-z0-9_.-]{0,127})$`)
var svgStyleSelector = regexp.MustCompile(`^[.#]?[A-Za-z_][A-Za-z0-9_-]*(?:\s*,\s*[.#]?[A-Za-z_][A-Za-z0-9_-]*)*$`)
var svgPlainValue = regexp.MustCompile(`^[A-Za-z0-9#.,%+\-\s"']{1,512}$`)
var svgColorFunction = regexp.MustCompile(`^(?:rgb|rgba|hsl|hsla)\([0-9.,%+\-\s/]+\)$`)
var svgGeometryValue = regexp.MustCompile(`^[A-Za-z0-9#.,%+\-\s():;]+$`)
var svgNumber = regexp.MustCompile(`[+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`)

func boundedSVGNumbers(value string, limit float64) bool {
	for _, number := range svgNumber.FindAllString(value, -1) {
		n, err := strconv.ParseFloat(number, 64)
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) || math.Abs(n) > limit {
			return false
		}
	}
	return true
}

// svgElements is the complete safe SVG element set: all shape, gradient,
// pattern, filter-primitive, animation, structural and descriptive elements.
// Anything capable of scripts, external resources or navigation is excluded:
// script, foreignObject, a, image, feImage, iframe, video, audio, handler,
// listener, prefetch, cursor, font-face-uri, font-face-src, animation, ...
var svgElements = []string{"svg", "g", "defs", "title", "desc", "metadata", "style", "path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "text", "tspan", "textPath", "linearGradient", "radialGradient", "stop", "pattern", "use", "clipPath", "mask", "marker", "symbol", "switch", "view", "hatch", "hatchpath", "solidcolor", "meshgradient", "mesh", "meshrow", "meshpatch", "font", "font-face", "font-face-name", "font-face-format", "glyph", "missing-glyph", "hkern", "vkern", "filter", "feBlend", "feColorMatrix", "feComponentTransfer", "feFuncA", "feFuncR", "feFuncG", "feFuncB", "feComposite", "feConvolveMatrix", "feDiffuseLighting", "feDisplacementMap", "feDistantLight", "feDropShadow", "feFlood", "feGaussianBlur", "feMerge", "feMergeNode", "feMorphology", "feOffset", "fePointLight", "feSpecularLighting", "feSpotLight", "feTile", "feTurbulence", "animate", "animateTransform", "animateMotion", "set", "mpath", "discard"}
var svgStyleProperty = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)
var svgDangerousStyleProperty = []string{"behavior", "binding", "-moz-binding", "-webkit-binding", "-ms-behavior"}

// stripSVGURLRefs removes url(...) segments, requiring each to be a local
// url(#id) reference; collected ids are appended to refs. Returns false on any
// non-local or malformed reference.
func stripSVGURLRefs(value string, refs *[]string) (string, bool) {
	for {
		i := strings.Index(strings.ToLower(value), "url(")
		if i < 0 {
			return value, true
		}
		j := strings.IndexByte(value[i:], ')')
		if j < 0 {
			return "", false
		}
		inner := strings.Trim(strings.TrimSpace(value[i+4:i+j]), `'"`)
		match := svgLocalRef.FindStringSubmatch(inner)
		if match == nil {
			return "", false
		}
		*refs = append(*refs, match[1])
		value = value[:i] + " " + value[i+j+1:]
	}
}

// svgValueDangerous rejects values that can carry URLs, schemes or CSS
// execution vectors no matter which attribute they appear in. A bare ':' kills
// every protocol scheme (javascript:, data:, file:, http:...); '//', '@', '{',
// '}', '<', '>' and '\\' never appear in legitimate attribute values.
func svgValueDangerous(value string) bool {
	if strings.ContainsAny(value, ":{}<>@\\`") || strings.Contains(value, "//") {
		return true
	}
	lower := strings.ToLower(value)
	return strings.Contains(lower, "expression(") || strings.Contains(lower, "eval(") || strings.Contains(lower, "-moz-binding")
}

var svgHexColor = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)

// checkSVGAttrValue returns "" for safe values or the rejection code. Local
// url(#id) refs are collected; everything else is a charset/number bound check.
func checkSVGAttrValue(value string, refs *[]string, numberBound float64) string {
	rest, ok := stripSVGURLRefs(value, refs)
	if !ok || svgValueDangerous(rest) {
		return "unsafe_svg"
	}
	// Strip hex colors before bounding numbers: "#2C3E50" otherwise parses
	// "3E50" as 3e50 and overflows the bound.
	rest = svgHexColor.ReplaceAllString(rest, "")
	if svgGeometryValue.MatchString(rest) && !boundedSVGNumbers(rest, numberBound) {
		return "svg_too_complex"
	}
	return ""
}

func safeStyleDeclarations(text string, refs *[]string) bool {
	if len(text) > 32768 || strings.ContainsAny(text, "\\@<>/*{}") {
		return false
	}
	for declaration := range strings.SplitSeq(text, ";") {
		if strings.TrimSpace(declaration) == "" {
			continue
		}
		property, value, ok := strings.Cut(declaration, ":")
		property = strings.TrimSpace(property)
		if !ok || !svgStyleProperty.MatchString(property) || slices.Contains(svgDangerousStyleProperty, strings.ToLower(property)) {
			return false
		}
		value = strings.TrimSpace(value)
		if len(value) > 1024 {
			return false
		}
		rest, ok := stripSVGURLRefs(value, refs)
		if !ok || svgValueDangerous(rest) {
			return false
		}
		rest = strings.TrimSpace(svgHexColor.ReplaceAllString(rest, ""))
		if rest == "" {
			continue
		}
		if svgGeometryValue.MatchString(rest) && !boundedSVGNumbers(rest, 8192) {
			return false
		}
		if !svgPlainValue.MatchString(rest) && !svgColorFunction.MatchString(rest) && !svgGeometryValue.MatchString(rest) {
			return false
		}
	}
	return true
}

func safeStyleSheet(text string, refs *[]string) bool {
	if len(text) > 32768 {
		return false
	}
	for strings.TrimSpace(text) != "" {
		selector, body, ok := strings.Cut(text, "{")
		if !ok || !svgStyleSelector.MatchString(strings.TrimSpace(selector)) {
			return false
		}
		declarations, rest, ok := strings.Cut(body, "}")
		if !ok || !safeStyleDeclarations(declarations, refs) {
			return false
		}
		text = rest
	}
	return true
}

func ExtractSafeSVG(text string) (string, string) {
	if len(text) > model.QualityMaxResponseBytes {
		return "", "response_too_large"
	}
	upper := strings.ToUpper(text)
	if strings.Contains(upper, "<!DOCTYPE") || strings.Contains(upper, "<!ENTITY") {
		return "", "unsafe_svg"
	}
	locations := svgOpening.FindAllStringIndex(text, -1)
	if len(locations) == 0 {
		return "", "missing_svg"
	}
	// Track element depth so a nested <svg> inside the first candidate does
	// not count as a second one; only a top-level <svg after the balanced
	// close is genuinely ambiguous.
	start := locations[0][0]
	depth := 0
	cursor := start
	end := -1
	for {
		openRel := svgOpening.FindStringIndex(text[cursor:])
		closeRel := strings.Index(text[cursor:], "</svg>")
		if closeRel < 0 {
			return "", "invalid_xml"
		}
		if openRel != nil && openRel[0] < closeRel {
			absOpen := cursor + openRel[0]
			tagEnd := strings.IndexByte(text[absOpen:], '>')
			selfClosing := tagEnd >= 0 && text[absOpen+tagEnd-1] == '/'
			if !selfClosing {
				depth++
			}
			if tagEnd >= 0 {
				cursor = absOpen + tagEnd + 1
			} else {
				cursor += openRel[1]
			}
			continue
		}
		depth--
		if depth == 0 {
			end = cursor + closeRel + len("</svg>")
			break
		}
		cursor += closeRel + len("</svg>")
	}
	if svgOpening.MatchString(text[end:]) {
		return "", "ambiguous_svg"
	}
	candidate := text[start:end]
	if len(candidate) > model.QualityMaxSVGBytes {
		return "", "svg_too_large"
	}
	decoder := xml.NewDecoder(strings.NewReader(candidate))
	decoder.Strict = true
	depth, nodes, drawing := 0, 0, 0
	ids := map[string]bool{}
	refs := []string{}
	styleDepth := 0
	var style strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "invalid_xml"
		}
		switch value := token.(type) {
		case xml.Directive, xml.ProcInst:
			return "", "unsafe_svg"
		case xml.StartElement:
			depth++
			nodes++
			if depth > 32 || nodes > 8192 {
				return "", "svg_too_complex"
			}
			if value.Name.Space != "http://www.w3.org/2000/svg" && value.Name.Space != "" {
				return "", "unsafe_svg"
			}
			if !slices.Contains(svgElements, value.Name.Local) || (depth == 1 && value.Name.Local != "svg") {
				return "", "unsafe_svg"
			}
			if styleDepth != 0 {
				return "", "unsafe_svg"
			}
			if value.Name.Local == "style" {
				styleDepth = depth
				style.Reset()
			}
			if slices.Contains([]string{"path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "text", "use", "textPath"}, value.Name.Local) {
				drawing++
			}
			if len(value.Attr) > 64 {
				return "", "svg_too_complex"
			}
			seenAttrs := map[string]bool{}
			for _, attr := range value.Attr {
				key := attr.Name.Space + ":" + attr.Name.Local
				if seenAttrs[key] {
					return "", "invalid_xml"
				}
				seenAttrs[key] = true
				name := attr.Name.Local
				if attr.Name.Space == "xmlns" {
					if name != "xlink" || attr.Value != "http://www.w3.org/1999/xlink" {
						return "", "unsafe_svg"
					}
					continue
				}
				if name == "xmlns" && attr.Name.Space == "" {
					if attr.Value != "http://www.w3.org/2000/svg" {
						return "", "unsafe_svg"
					}
					continue
				}
				if name == "href" && (attr.Name.Space == "" || attr.Name.Space == "xlink" || attr.Name.Space == "http://www.w3.org/1999/xlink") {
					match := svgLocalRef.FindStringSubmatch(attr.Value)
					if match == nil {
						return "", "unsafe_svg"
					}
					refs = append(refs, match[1])
					continue
				}
				if name == "space" && attr.Name.Space == "http://www.w3.org/XML/1998/namespace" && (attr.Value == "default" || attr.Value == "preserve") {
					continue
				}
				if attr.Name.Space != "" {
					return "", "unsafe_svg"
				}
				if name == "id" {
					if !svgIdentifier.MatchString(attr.Value) {
						return "", "unsafe_svg"
					}
					if ids[attr.Value] {
						return "", "invalid_xml"
					}
					ids[attr.Value] = true
					continue
				}
				if name == "class" {
					if len(attr.Value) > 512 {
						return "", "svg_too_complex"
					}
					for _, class := range strings.Fields(attr.Value) {
						if !svgIdentifier.MatchString(class) {
							return "", "unsafe_svg"
						}
					}
					continue
				}
				if name == "style" {
					if !safeStyleDeclarations(attr.Value, &refs) {
						return "", "unsafe_svg"
					}
					continue
				}
				if name == "type" && value.Name.Local == "style" && attr.Value == "text/css" {
					continue
				}
				if name == "role" && attr.Value == "img" {
					continue
				}
				if name == "aria-label" && len(attr.Value) <= 512 {
					continue
				}
				if name == "aria-labelledby" || name == "aria-describedby" {
					fields := strings.Fields(attr.Value)
					if len(fields) == 0 || len(attr.Value) > 512 {
						return "", "unsafe_svg"
					}
					for _, ref := range fields {
						if !svgIdentifier.MatchString(ref) {
							return "", "unsafe_svg"
						}
						refs = append(refs, ref)
					}
					continue
				}
				if (name == "aria-hidden" || name == "focusable") && (attr.Value == "true" || attr.Value == "false") {
					continue
				}
				// Event handlers are always unsafe regardless of the element.
				if strings.HasPrefix(strings.ToLower(name), "on") {
					return "", "unsafe_svg"
				}
				// Generic attributes: the name vocabulary is unbounded, so the
				// value is constrained instead — local url(#id) refs only, no
				// schemes/delimiters, bounded numbers, generous length caps for
				// path data.
				limit := 1024
				if name == "d" || name == "points" || name == "path" {
					limit = 65536
				}
				if len(attr.Value) > limit {
					return "", "svg_too_complex"
				}
				maxNumber := float64(1_000_000)
				if depth == 1 && (name == "width" || name == "height") {
					maxNumber = 8192
				}
				if code := checkSVGAttrValue(attr.Value, &refs, maxNumber); code != "" {
					return "", code
				}
			}
		case xml.EndElement:
			if styleDepth == depth {
				if !safeStyleSheet(style.String(), &refs) {
					return "", "unsafe_svg"
				}
				styleDepth = 0
			}
			depth--
		case xml.CharData:
			if styleDepth != 0 {
				style.Write(value)
			}
		}
	}
	if depth != 0 || nodes == 0 {
		return "", "invalid_xml"
	}
	if drawing == 0 {
		return "", "empty_svg"
	}
	for _, ref := range refs {
		if !ids[ref] {
			return "", "unsafe_svg"
		}
	}
	return candidate, ""
}
