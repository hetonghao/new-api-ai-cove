package qualityinspect

import "github.com/QuantumNous/new-api/model"

// SVGOutputContract is the fixed platform output contract sent to the model for
// every svg test case. It encodes what the extractor and thumbnail renderer
// require, so case instructions only carry case-specific needs. Bump
// SVGOutputContractVersion whenever the contract text changes so historical
// runs remain interpretable.
const SVGOutputContractVersion = "v1"

const SVGOutputContract = `Respond with a single SVG image only. Requirements:
- output must be well-formed XML with exactly one root <svg xmlns="http://www.w3.org/2000/svg"> element
- the root <svg> must declare viewBox="0 0 W H" with numeric bounds (width/height recommended)
- escape &, <, > inside attribute values and close every tag properly
- XML comments must not contain "--"
- no markdown fences, no explanation, no comments, no second SVG document`

// composeInstruction prepends the fixed platform contract for svg cases so the
// case instruction only needs to carry case-specific requirements.
func composeInstruction(cfg model.QualityConfig) string {
	instruction := cfg.Instruction
	if cfg.OutputType != "svg" {
		return instruction
	}
	if instruction == "" {
		return SVGOutputContract
	}
	return SVGOutputContract + "\n\n" + instruction
}
