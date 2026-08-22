package athq

import "testing"

func TestParseS3URI(t *testing.T) {
	bucket, key, err := parseS3URI("s3://my-bucket/results/2026/01/abc.csv")
	if err != nil {
		t.Fatalf("got error %v, want none", err)
	}
	if bucket != "my-bucket" {
		t.Errorf("bucket: got = %q, want %q", bucket, "my-bucket")
	}
	if key != "results/2026/01/abc.csv" {
		t.Errorf("key: got = %q, want %q", key, "results/2026/01/abc.csv")
	}
}

func TestParseS3URIRejectsOtherSchemes(t *testing.T) {
	if _, _, err := parseS3URI("https://example.com/x.csv"); err == nil {
		t.Error("got no error for an https URI, want one")
	}
}

func TestParseS3URIRejectsMissingKey(t *testing.T) {
	if _, _, err := parseS3URI("s3://my-bucket"); err == nil {
		t.Error("got no error for a bucket without a key, want one")
	}
}
