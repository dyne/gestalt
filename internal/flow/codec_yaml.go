package flow

import (
	"bytes"
	"encoding/json"
	"fmt"

	internalschema "gestalt/internal/schema"
	"gopkg.in/yaml.v3"
)

// DecodeFlowBundleYAML decodes, schema-validates, and semantically validates a YAML flow bundle.
func DecodeFlowBundleYAML(data []byte, activityDefs []ActivityDef) (Config, error) {
	object, err := decodeYAMLObject(data)
	if err != nil {
		return DefaultConfig(), err
	}

	s, err := internalschema.Resolve(SchemaFlowBundle)
	if err != nil {
		return DefaultConfig(), err
	}
	if err := internalschema.ValidateObject(s, object); err != nil {
		return DefaultConfig(), err
	}

	dto, err := decodeBundleDTO(object)
	if err != nil {
		return DefaultConfig(), err
	}
	cfg := flowBundleToConfig(dto)
	if err := ValidateConfig(cfg, activityDefs); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func decodeFlowFileYAML(data []byte) (FlowFile, error) {
	object, err := decodeYAMLObject(data)
	if err != nil {
		return FlowFile{}, err
	}
	s, err := internalschema.Resolve(SchemaFlowFile)
	if err != nil {
		return FlowFile{}, err
	}
	if err := internalschema.ValidateObject(s, object); err != nil {
		return FlowFile{}, err
	}

	payload, err := json.Marshal(object)
	if err != nil {
		return FlowFile{}, err
	}
	var dto FlowFile
	if err := json.Unmarshal(payload, &dto); err != nil {
		return FlowFile{}, err
	}
	return dto, nil
}

func encodeFlowFileYAML(flowFile FlowFile) ([]byte, error) {
	return yaml.Marshal(flowFile)
}

// EncodeFlowBundleYAML converts domain config to YAML flow bundle payload.
func EncodeFlowBundleYAML(cfg Config) ([]byte, error) {
	dto := configToFlowBundle(cfg)
	payload, err := yaml.Marshal(dto)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeYAMLObject(data []byte) (map[string]any, error) {
	var object map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("invalid YAML flow payload: %w", err)
	}
	return object, nil
}

func decodeBundleDTO(object map[string]any) (FlowBundle, error) {
	payload, err := json.Marshal(object)
	if err != nil {
		return FlowBundle{}, err
	}
	var dto FlowBundle
	if err := json.Unmarshal(payload, &dto); err != nil {
		return FlowBundle{}, err
	}
	return dto, nil
}

func configToFlowBundle(cfg Config) FlowBundle {
	bundle := FlowBundle{
		Version: cfg.Version,
		Flows:   make([]FlowFile, 0, len(cfg.Triggers)),
	}
	if bundle.Version == 0 {
		bundle.Version = ConfigVersion
	}
	for _, trigger := range cfg.Triggers {
		flowFile := FlowFile{
			ID:        trigger.ID,
			Label:     trigger.Label,
			EventType: trigger.EventType,
			Where:     FlowWhere{},
			Bindings:  []FlowBinding{},
		}
		for key, value := range trigger.Where {
			flowFile.Where[key] = value
		}
		for _, binding := range cfg.BindingsByTriggerID[trigger.ID] {
			flowBinding := FlowBinding{
				ActivityID: binding.ActivityID,
				Config:     FlowBindingConfig{},
			}
			for key, value := range binding.Config {
				flowBinding.Config[key] = value
			}
			flowFile.Bindings = append(flowFile.Bindings, flowBinding)
		}
		bundle.Flows = append(bundle.Flows, flowFile)
	}
	return bundle
}

func flowBundleToConfig(bundle FlowBundle) Config {
	cfg := DefaultConfig()
	cfg.Version = bundle.Version
	if cfg.Version == 0 {
		cfg.Version = ConfigVersion
	}
	for _, item := range bundle.Flows {
		trigger := EventTrigger{
			ID:        item.ID,
			Label:     item.Label,
			EventType: item.EventType,
			Where:     map[string]string{},
		}
		for key, value := range item.Where {
			trigger.Where[key] = value
		}
		cfg.Triggers = append(cfg.Triggers, trigger)
		bindings := make([]ActivityBinding, 0, len(item.Bindings))
		for _, binding := range item.Bindings {
			activity := ActivityBinding{
				ActivityID: binding.ActivityID,
				Config:     map[string]any{},
			}
			for key, value := range binding.Config {
				activity.Config[key] = value
			}
			bindings = append(bindings, activity)
		}
		cfg.BindingsByTriggerID[item.ID] = bindings
	}
	return cfg
}
