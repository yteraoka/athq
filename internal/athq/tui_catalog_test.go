package athq

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
)

func TestPartitionProjectionIsEmptyWhenNotEnabled(t *testing.T) {
	params := map[string]string{
		"projection.dt.type":   "date",
		"projection.dt.format": "yyyy/MM/dd",
	}
	if got := partitionProjection(params, "dt"); got != "" {
		t.Errorf("got = %q, want empty without projection.enabled=true", got)
	}
}

func TestPartitionProjectionIsEmptyWithoutAColumnType(t *testing.T) {
	params := map[string]string{"projection.enabled": "true"}
	if got := partitionProjection(params, "dt"); got != "" {
		t.Errorf("got = %q, want empty without a projection type for the column", got)
	}
}

func TestPartitionProjectionDate(t *testing.T) {
	params := map[string]string{
		"projection.enabled":   "true",
		"projection.dt.type":   "date",
		"projection.dt.format": "yyyy/MM/dd",
		"projection.dt.range":  "2020/01/01,NOW",
	}
	if got := partitionProjection(params, "dt"); got != "date yyyy/MM/dd" {
		t.Errorf("got = %q, want %q", got, "date yyyy/MM/dd")
	}
}

func TestPartitionProjectionDateWithoutAFormatFallsBackToTheType(t *testing.T) {
	params := map[string]string{
		"projection.enabled": "true",
		"projection.dt.type": "date",
	}
	if got := partitionProjection(params, "dt"); got != "date" {
		t.Errorf("got = %q, want %q", got, "date")
	}
}

func TestPartitionProjectionInteger(t *testing.T) {
	params := map[string]string{
		"projection.enabled":       "true",
		"projection.year.type":     "integer",
		"projection.year.range":    "2020,2030",
		"projection.year.interval": "1",
	}
	if got := partitionProjection(params, "year"); got != "integer 2020–2030" {
		t.Errorf("got = %q, want %q", got, "integer 2020–2030")
	}
}

func TestPartitionProjectionEnum(t *testing.T) {
	params := map[string]string{
		"projection.enabled":      "true",
		"projection.stage.type":   "enum",
		"projection.stage.values": "dev,staging,prod",
	}
	if got := partitionProjection(params, "stage"); got != "enum dev, staging, prod" {
		t.Errorf("got = %q, want %q", got, "enum dev, staging, prod")
	}
}

func TestPartitionProjectionInjected(t *testing.T) {
	params := map[string]string{
		"projection.enabled":     "true",
		"projection.custom.type": "injected",
	}
	if got := partitionProjection(params, "custom"); got != "injected" {
		t.Errorf("got = %q, want %q", got, "injected")
	}
}

func TestPartitionProjectionIsPerColumn(t *testing.T) {
	params := map[string]string{
		"projection.enabled":   "true",
		"projection.dt.type":   "date",
		"projection.dt.format": "yyyy/MM/dd",
	}
	if got := partitionProjection(params, "other"); got != "" {
		t.Errorf("got = %q, want empty for a column with no projection of its own", got)
	}
}

func TestToTUIColumnsCarriesTheProjectionOnPartitionKeysOnly(t *testing.T) {
	m := types.TableMetadata{
		Columns: []types.Column{
			{Name: aws.String("event_id"), Type: aws.String("varchar")},
		},
		PartitionKeys: []types.Column{
			{Name: aws.String("dt"), Type: aws.String("string")},
		},
		Parameters: map[string]string{
			"projection.enabled":   "true",
			"projection.dt.type":   "date",
			"projection.dt.format": "yyyy/MM/dd",
		},
	}

	cols := toTUIColumns(m)
	if len(cols) != 2 {
		t.Fatalf("got %d columns, want 2", len(cols))
	}
	if cols[0].projection != "" {
		t.Errorf("ordinary column: got projection = %q, want empty", cols[0].projection)
	}
	if !cols[1].partition || cols[1].projection != "date yyyy/MM/dd" {
		t.Errorf("partition column: got partition=%v projection=%q, want true and %q",
			cols[1].partition, cols[1].projection, "date yyyy/MM/dd")
	}
}

func TestToTUIColumnsLeavesTheProjectionEmptyForAPlainHivePartition(t *testing.T) {
	m := types.TableMetadata{
		PartitionKeys: []types.Column{
			{Name: aws.String("dt"), Type: aws.String("string")},
		},
	}

	cols := toTUIColumns(m)
	if len(cols) != 1 || cols[0].projection != "" {
		t.Errorf("got = %+v, want a partition column with no projection", cols)
	}
}
