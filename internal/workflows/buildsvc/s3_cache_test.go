package buildsvc

import "testing"

func TestS3CacheSpecsFromURL(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")

	from, to, err := S3CacheSpecs(S3CacheOptions{
		Ref:          "s3://torque-cache/prefix/app",
		Name:         "ghcr.io/acme/api:dev",
		Mode:         "max",
		EndpointURL:  "https://s3.example.test",
		UsePathStyle: true,
	}, "")
	if err != nil {
		t.Fatalf("S3CacheSpecs returned error: %v", err)
	}
	wantFrom := "type=s3,bucket=torque-cache,region=us-east-1,prefix=prefix/app/,name=ghcr.io-acme-api-dev,endpoint_url=https://s3.example.test,use_path_style=true"
	if from != wantFrom {
		t.Fatalf("from spec mismatch:\n got: %s\nwant: %s", from, wantFrom)
	}
	wantTo := wantFrom + ",mode=max"
	if to != wantTo {
		t.Fatalf("to spec mismatch:\n got: %s\nwant: %s", to, wantTo)
	}
	if _, err := ParseCacheSpecs([]string{from, to}); err != nil {
		t.Fatalf("generated specs should parse: %v", err)
	}
}

func TestApplyS3CacheAppendsImportAndExport(t *testing.T) {
	from, to, err := ApplyS3Cache(
		[]string{"type=registry,ref=example/cache"},
		nil,
		S3CacheOptions{Ref: "bucket/cache", Region: "eu-west-1", Mode: "min"},
		"api",
	)
	if err != nil {
		t.Fatalf("ApplyS3Cache returned error: %v", err)
	}
	if len(from) != 2 {
		t.Fatalf("cache-from length = %d, want 2: %#v", len(from), from)
	}
	if len(to) != 1 {
		t.Fatalf("cache-to length = %d, want 1: %#v", len(to), to)
	}
	if got := from[1]; got != "type=s3,bucket=bucket,region=eu-west-1,prefix=cache/,name=api" {
		t.Fatalf("unexpected generated cache-from: %s", got)
	}
	if got := to[0]; got != "type=s3,bucket=bucket,region=eu-west-1,prefix=cache/,name=api,mode=min" {
		t.Fatalf("unexpected generated cache-to: %s", got)
	}
}

func TestS3CacheSpecsRejectsBadMode(t *testing.T) {
	_, _, err := S3CacheSpecs(S3CacheOptions{Ref: "bucket/cache", Region: "us-east-1", Mode: "full"}, "api")
	if err == nil {
		t.Fatalf("expected invalid mode error")
	}
}
