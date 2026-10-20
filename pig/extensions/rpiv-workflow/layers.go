package workflow

// ConfigLayer names the layer a workflow definition came from — the shared
// vocabulary of the loader (merge precedence) and the validator (issue
// attribution).
type ConfigLayer string

// The three layers, in ascending precedence.
const (
	LayerBuiltIn ConfigLayer = "built-in"
	LayerUser    ConfigLayer = "user"
	LayerProject ConfigLayer = "project"
)

// configLayerLabels is the single source for layer → display string. The
// current display form is the layer name verbatim — keep that invariant.
var configLayerLabels = map[ConfigLayer]string{
	LayerBuiltIn: "built-in",
	LayerUser:    "user",
	LayerProject: "project",
}

// RenderConfigLayer is the one interpolation site for a layer's display form
// (banner builders, issue prefixes). An unknown layer renders as itself.
func RenderConfigLayer(layer ConfigLayer) string {
	if label, ok := configLayerLabels[layer]; ok {
		return label
	}
	return string(layer)
}
