package athq

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
)

// outputSetting is where the results of a query will be written, and what
// decided it. Neither is obvious from the command line: without
// --output-location the work group's own setting is used, and a work group
// that enforces its configuration ignores the flag altogether.
type outputSetting struct {
	location string
	source   string
}

type msgTUIWorkGroup struct {
	output outputSetting
	err    error
}

func loadWorkGroupCmd(ctx context.Context, c *clients) tea.Cmd {
	return func() tea.Msg {
		name := workGroup()
		out, err := c.athena.GetWorkGroup(ctx, &athena.GetWorkGroupInput{WorkGroup: aws.String(name)})
		if err != nil {
			return msgTUIWorkGroup{err: fmt.Errorf("failed to get the work group %s: %w", name, err)}
		}
		var cfg *types.WorkGroupConfiguration
		if out.WorkGroup != nil {
			cfg = out.WorkGroup.Configuration
		}
		return msgTUIWorkGroup{output: resolveOutputLocation(outputLocation(), cfg)}
	}
}

// resolveOutputLocation works out which of the two settings wins.
func resolveOutputLocation(override string, cfg *types.WorkGroupConfiguration) outputSetting {
	var groupLocation string
	var enforced bool
	if cfg != nil {
		if cfg.ResultConfiguration != nil {
			groupLocation = aws.ToString(cfg.ResultConfiguration.OutputLocation)
		}
		enforced = aws.ToBool(cfg.EnforceWorkGroupConfiguration)
	}

	// The work group's own location needs no explanation, since the header
	// names the work group next to it; the other two cases do.
	switch {
	case override != "" && enforced && groupLocation != "":
		return outputSetting{location: groupLocation, source: outputLocationSource() + " ignored"}
	case override != "":
		return outputSetting{location: override, source: outputLocationSource()}
	case groupLocation != "":
		return outputSetting{location: groupLocation}
	default:
		return outputSetting{}
	}
}

// outputLocationSource names whichever of the two ways set the location, so
// the header can say where the value came from.
func outputLocationSource() string {
	if opts.outputLocation != "" {
		return "--output-location"
	}
	return envOutputLocation
}
