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
var svgLocalPaint = regexp.MustCompile(`^url\(\s*#([A-Za-z_][A-Za-z0-9_.-]{0,127})\s*\)$`)
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

var svgElements = []string{"svg", "g", "defs", "title", "desc", "path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "text", "tspan", "textPath", "linearGradient", "radialGradient", "stop", "style", "pattern", "use", "clipPath", "mask", "marker", "symbol", "filter", "feGaussianBlur", "feDropShadow", "feOffset", "feFlood", "feColorMatrix", "feBlend", "feComposite", "feMerge", "feMergeNode", "feMorphology", "feTurbulence", "feDisplacementMap", "feConvolveMatrix", "feDiffuseLighting", "feSpecularLighting", "feDistantLight", "fePointLight", "feSpotLight", "feTile", "animate", "animateTransform", "animateMotion", "set", "mpath"}
var svgPaintProperties = []string{"fill", "stroke", "stroke-width", "stroke-linecap", "stroke-linejoin", "stroke-miterlimit", "fill-rule", "opacity", "fill-opacity", "stroke-opacity", "font-family", "font-size", "font-weight", "font-style", "text-anchor", "dominant-baseline", "letter-spacing", "word-spacing", "paint-order", "stroke-dasharray", "stroke-dashoffset", "stop-color", "stop-opacity", "marker-start", "marker-mid", "marker-end", "clip-path", "mask", "filter", "color", "vector-effect", "shape-rendering", "text-rendering", "image-rendering", "display", "visibility", "transform", "transform-origin", "mask-type", "flood-color", "flood-opacity", "lighting-color"}
var svgURLPaintProperties = []string{"fill", "stroke", "marker-start", "marker-mid", "marker-end", "clip-path", "mask", "filter"}
var svgGeometryAttributes = []string{"viewBox", "preserveAspectRatio", "x", "y", "z", "x1", "y1", "x2", "y2", "dx", "dy", "cx", "cy", "r", "rx", "ry", "width", "height", "d", "points", "path", "gradientTransform", "gradientUnits", "spreadMethod", "offset", "fx", "fy", "fr", "rotate", "textLength", "lengthAdjust", "version", "patternUnits", "patternContentUnits", "patternTransform", "clipPathUnits", "maskUnits", "maskContentUnits", "markerUnits", "markerWidth", "markerHeight", "refX", "refY", "orient", "filterUnits", "primitiveUnits", "in", "in2", "result", "stdDeviation", "type", "values", "mode", "scale", "baseFrequency", "numOctaves", "seed", "startOffset", "method", "spacing", "side", "slope", "intercept", "amplitude", "exponent", "tableValues", "kernelMatrix", "kernelUnitLength", "order", "targetX", "targetY", "stitchTiles", "radius", "k1", "k2", "k3", "k4", "operator", "surfaceScale", "diffuseConstant", "specularConstant", "specularExponent", "limitingConeAngle", "azimuth", "elevation", "pointsAtX", "pointsAtY", "pointsAtZ", "keyPoints", "keyTimes", "keySplines", "from", "to", "by", "additive", "accumulate", "attributeName", "attributeType", "begin", "dur", "end", "min", "max", "restart", "repeatCount", "repeatDur", "calcMode", "xChannelSelector", "yChannelSelector", "divisor", "bias", "edgeMode", "preserveAlpha"}

func safePaintValue(property, value string, refs *[]string) bool {
	if !slices.Contains(svgPaintProperties, property) {
		return false
	}
	value = strings.TrimSpace(value)
	if match := svgLocalPaint.FindStringSubmatch(value); match != nil {
		if !slices.Contains(svgURLPaintProperties, property) {
			return false
		}
		*refs = append(*refs, match[1])
		return true
	}
	if property == "transform" || property == "transform-origin" {
		return len(value) <= 1024 && svgGeometryValue.MatchString(value) && boundedSVGNumbers(value, 1_000_000)
	}
	if !slices.Contains([]string{"fill", "stroke", "stop-color", "font-family"}, property) && !boundedSVGNumbers(value, 8192) {
		return false
	}
	return svgPlainValue.MatchString(value) || (len(value) <= 128 && svgColorFunction.MatchString(value))
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
		if !ok || !safePaintValue(strings.TrimSpace(property), value, refs) {
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
	if len(locations) != 1 {
		return "", "ambiguous_svg"
	}
	start := locations[0][0]
	end := strings.Index(text[start:], "</svg>")
	if end < 0 {
		return "", "invalid_xml"
	}
	end += start + len("</svg>")
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
			if !slices.Contains(svgElements, value.Name.Local) || (depth == 1 && value.Name.Local != "svg") || (depth > 1 && value.Name.Local == "svg") {
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
				if slices.Contains(svgPaintProperties, name) {
					if !safePaintValue(name, attr.Value, &refs) {
						return "", "unsafe_svg"
					}
					continue
				}
				if slices.Contains(svgGeometryAttributes, name) {
					limit := 1024
					if name == "d" || name == "points" || name == "path" {
						limit = 65536
					}
					maxNumber := float64(1_000_000)
					if depth == 1 && (name == "width" || name == "height") {
						maxNumber = 8192
					}
					if len(attr.Value) > limit || !svgGeometryValue.MatchString(attr.Value) || !boundedSVGNumbers(attr.Value, maxNumber) {
						return "", "svg_too_complex"
					}
					continue
				}
				return "", "unsafe_svg"
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
