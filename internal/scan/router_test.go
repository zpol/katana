package scan_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecr "github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/zpol/katana/internal/ecr"
	"github.com/zpol/katana/internal/jfrog"
	"github.com/zpol/katana/internal/scan"
	"github.com/zpol/katana/internal/store"
)

type fakeECR struct {
	out *awsecr.DescribeImageScanFindingsOutput
	err error
}

func (f *fakeECR) DescribeImageScanFindings(ctx context.Context, params *awsecr.DescribeImageScanFindingsInput, optFns ...func(*awsecr.Options)) (*awsecr.DescribeImageScanFindingsOutput, error) {
	return f.out, f.err
}

func TestRouterRoutesNonECRToJFrog(t *testing.T) {
	var cfg store.IntegrationsSettings
	cfg.Scanners.JFrog.Enabled = true
	cfg.Scanners.ECRInspector.Enabled = true
	r := &scan.Router{
		Config: scan.StaticConfig{Cfg: cfg},
		JFrog:  jfrog.NewHTTPClient("", ""),
		ECR:    ecr.NewClient("eu-west-1"),
	}
	sum, err := r.GetScanSummary(context.Background(), scan.ArtifactRef{ImageRef: "evil.example/app:1"})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scanner != "jfrog" {
		t.Fatalf("scanner=%q", sum.Scanner)
	}
}

func TestRouterRoutesECR(t *testing.T) {
	var cfg store.IntegrationsSettings
	cfg.Scanners.ECRInspector.Enabled = true
	api := &fakeECR{out: &awsecr.DescribeImageScanFindingsOutput{
		ImageScanStatus: &types.ImageScanStatus{Status: types.ScanStatusComplete},
		ImageScanFindings: &types.ImageScanFindings{
			FindingSeverityCounts: map[string]int32{"HIGH": 1},
			Findings:              []types.ImageScanFinding{{Name: aws.String("CVE-1"), Severity: types.FindingSeverityHigh}},
		},
	}}
	r := &scan.Router{
		Config: scan.StaticConfig{Cfg: cfg},
		ECR:    ecr.NewClientWithAPI("eu-west-1", api),
	}
	sum, err := r.GetScanSummary(context.Background(), scan.ArtifactRef{
		ImageRef: "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scanner != "ecr_inspector" || !sum.Scanned || sum.High != 1 {
		t.Fatalf("%+v", sum)
	}
}

func TestRouterDisabledECR(t *testing.T) {
	var cfg store.IntegrationsSettings
	r := &scan.Router{
		Config: scan.StaticConfig{Cfg: cfg},
		ECR:    ecr.NewClientWithAPI("eu-west-1", &fakeECR{err: errors.New("should not call")}),
	}
	sum, err := r.GetScanSummary(context.Background(), scan.ArtifactRef{
		ImageRef: "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.LookupStatus != scan.StatusUnavailable || !strings.Contains(sum.RawError, "disabled") {
		t.Fatalf("%+v", sum)
	}
}
