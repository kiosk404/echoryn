package plugin

// InTreeRegistry is a pre-configured set of built-in plugin factories.
// This mirrors K8s scheduler's in-tree plugin registration pattern,
// where all default plugins are registered in a single function.
//
// Out-of-tree plugins can be added via Framework.RegisterFactory() directly.
type InTreeRegistry struct {
	entries []inTreeEntry
}

type inTreeEntry struct {
	def     Definition
	factory PluginFactory
	args    PluginArgs
}

// NewInTreeRegistry creates a new in-tree plugin registry.
func NewInTreeRegistry() *InTreeRegistry {
	return &InTreeRegistry{}
}

// Register adds a plugin factory to the in-tree registry.
func (r *InTreeRegistry) Register(def Definition, factory PluginFactory, args PluginArgs) {
	r.entries = append(r.entries, inTreeEntry{
		def:     def,
		factory: factory,
		args:    args,
	})
}

// Len returns the number of registered factories.
func (r *InTreeRegistry) Len() int {
	return len(r.entries)
}

// ApplyTo registers all in-tree plugin factories into the given Framework.
func (r *InTreeRegistry) ApplyTo(f *Framework) error {
	for _, entry := range r.entries {
		if err := f.RegisterFactory(entry.def, entry.factory, entry.args); err != nil {
			return err
		}
	}
	return nil
}
