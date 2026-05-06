package tool

import (
	"github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/connectors/tools/github"
)

type Tools map[string]runtime.ToolConnector

type Factory func() runtime.ToolConnector

var ToolFactories = map[string]Factory{
	"github": func() runtime.ToolConnector { return github.NewConnector(nil) },
}

func DefaultTools() Tools {
	return BuildTools(nil)
}

func BuildTools(enabled []string) Tools {
	out := make(Tools)
	if len(enabled) == 0 {
		for name, factory := range ToolFactories {
			out[name] = factory()
		}
		return out
	}
	for _, name := range enabled {
		if factory, ok := ToolFactories[name]; ok {
			out[name] = factory()
		}
	}
	return out
}
