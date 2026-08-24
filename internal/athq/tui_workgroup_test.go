package athq

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
)

func workGroupConfig(location string, enforced bool) *types.WorkGroupConfiguration {
	cfg := &types.WorkGroupConfiguration{EnforceWorkGroupConfiguration: aws.Bool(enforced)}
	if location != "" {
		cfg.ResultConfiguration = &types.ResultConfiguration{OutputLocation: aws.String(location)}
	}
	return cfg
}

func TestResolveOutputLocation(t *testing.T) {
	t.Setenv(envOutputLocation, "")
	opts.outputLocation = ""

	tests := []struct {
		name     string
		override string
		cfg      *types.WorkGroupConfiguration
		want     outputSetting
	}{
		{
			name: "the work group setting is used on its own",
			cfg:  workGroupConfig("s3://wg/", false),
			want: outputSetting{location: "s3://wg/"},
		},
		{
			name:     "an override wins",
			override: "s3://mine/",
			cfg:      workGroupConfig("s3://wg/", false),
			want:     outputSetting{location: "s3://mine/", source: envOutputLocation},
		},
		{
			name:     "an enforcing work group ignores the override",
			override: "s3://mine/",
			cfg:      workGroupConfig("s3://wg/", true),
			want:     outputSetting{location: "s3://wg/", source: envOutputLocation + " ignored"},
		},
		{
			name:     "an enforcing work group without a location leaves the override",
			override: "s3://mine/",
			cfg:      workGroupConfig("", true),
			want:     outputSetting{location: "s3://mine/", source: envOutputLocation},
		},
		{
			name: "nothing is set anywhere",
			cfg:  workGroupConfig("", false),
			want: outputSetting{},
		},
		{
			name: "the work group has no configuration at all",
			cfg:  nil,
			want: outputSetting{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOutputLocation(tt.override, tt.cfg); got != tt.want {
				t.Errorf("got = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOutputLocationSourceNamesTheFlag(t *testing.T) {
	opts.outputLocation = "s3://mine/"
	defer func() { opts.outputLocation = "" }()

	if got := outputLocationSource(); got != "--output-location" {
		t.Errorf("got = %q, want the flag", got)
	}
}

func TestHeaderShowsTheWorkGroupAndTheOutputLocation(t *testing.T) {
	m := loadedTUI(t)

	next, _ := m.Update(msgTUIWorkGroup{output: resolveOutputLocation("", workGroupConfig("s3://results/prefix/", false))})
	m = next.(tuiModel)

	header := stripANSI(strings.Split(m.View().Content, "\n")[0])
	for _, want := range []string{"work group: " + testWorkGroup, "output: s3://results/prefix/"} {
		if !strings.Contains(header, want) {
			t.Errorf("the header %q does not contain %q", header, want)
		}
	}
}

func TestHeaderNamesTheOverrideThatSetTheLocation(t *testing.T) {
	m := loadedTUI(t)

	next, _ := m.Update(msgTUIWorkGroup{output: resolveOutputLocation("s3://mine/", workGroupConfig("s3://wg/", false))})
	m = next.(tuiModel)

	header := stripANSI(strings.Split(m.View().Content, "\n")[0])
	if !strings.Contains(header, "s3://mine/ ("+envOutputLocation+")") {
		t.Errorf("the header %q does not say where the location came from", header)
	}
}

func TestHeaderWhileTheWorkGroupIsBeingRead(t *testing.T) {
	m := loadedTUI(t)
	if !m.wgLoading {
		t.Fatal("got wgLoading = false, want the work group being read")
	}
	header := stripANSI(strings.Split(m.View().Content, "\n")[0])
	if !strings.Contains(header, "work group: "+testWorkGroup) {
		t.Errorf("the header %q does not name the work group", header)
	}
	if !strings.Contains(header, "output: …") {
		t.Errorf("the header %q does not show the location being read", header)
	}
}

func TestHeaderSaysWhenTheWorkGroupCannotBeRead(t *testing.T) {
	m := loadedTUI(t)

	next, _ := m.Update(msgTUIWorkGroup{err: errTest})
	m = next.(tuiModel)

	header := stripANSI(strings.Split(m.View().Content, "\n")[0])
	if !strings.Contains(header, "unknown") {
		t.Errorf("the header %q does not say the location is unknown", header)
	}
	if !m.statusErr || !strings.Contains(m.status, errTest.Error()) {
		t.Errorf("status: got = %q (err=%v), want the reason", m.status, m.statusErr)
	}
}

func TestHeaderKeepsTheExplicitLocationWhenTheWorkGroupCannotBeRead(t *testing.T) {
	m := loadedTUI(t)
	t.Setenv(envOutputLocation, "s3://mine/")

	next, _ := m.Update(msgTUIWorkGroup{err: errTest})
	m = next.(tuiModel)

	header := stripANSI(strings.Split(m.View().Content, "\n")[0])
	if !strings.Contains(header, "s3://mine/") {
		t.Errorf("the header %q does not show the location that was given explicitly", header)
	}
}

func TestHeaderSaysWhenNoLocationIsSetAnywhere(t *testing.T) {
	m := loadedTUI(t)

	next, _ := m.Update(msgTUIWorkGroup{output: resolveOutputLocation("", workGroupConfig("", false))})
	m = next.(tuiModel)

	header := stripANSI(strings.Split(m.View().Content, "\n")[0])
	if !strings.Contains(header, "not set") {
		t.Errorf("the header %q does not say that no location is set", header)
	}
}
