package plugin

import (
	"fmt"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// SlotConfig maps slot kind → desired plugin name.
// For example: {"memory": "memory-core"} means only the "memory-core" plugin
// should be active for the "memory" slot.
//
// Special values:
//   - "none": disable all plugins of this kind
//   - "": use the default plugin for this kind
type SlotConfig map[string]string

// slotDefaults defines the default active plugin for each slot kind.
// This corresponds to OpenClaw's default slot selections.
var slotDefaults = map[string]string{
	"memory": "memory-core",
}

// ResolveSlot determines whether a plugin should be activated based on
// its Kind and the slot configuration.
//
// Returns nil if the plugin is allowed; returns an error (with explanation)
// if the plugin should be skipped.
//
// This implements OpenClaw's slot exclusion mechanism where only one plugin
// per Kind can be active at a time.
func ResolveSlot(def Definition, activeSlots map[string]string, config SlotConfig) error {
	kind := def.Kind
	if kind == "" || kind == "general" {
		return nil // No slot constraint.
	}

	// Determine the desired plugin for this kind.
	desired := config[kind]
	if desired == "" {
		desired = slotDefaults[kind]
	}

	// "none" means disable all plugins of this kind.
	if desired == "none" {
		return fmt.Errorf("slot %q is disabled by configuration", kind)
	}

	// Check if this plugin is the desired one.
	if desired != def.ID {
		return fmt.Errorf("slot %q is assigned to %q, skipping %q", kind, desired, def.ID)
	}

	// Check if another plugin already occupies this slot.
	if occupant, occupied := activeSlots[kind]; occupied && occupant != def.ID {
		return fmt.Errorf("slot %q already occupied by %q, cannot load %q", kind, occupant, def.ID)
	}

	logger.Info("[Plugin] slot %q assigned to plugin %q", kind, def.ID)
	return nil
}
