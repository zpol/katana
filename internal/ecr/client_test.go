package ecr

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecr "github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/zpol/katana/internal/scan"
)

type fakeAPI struct {
	out *awsecr.DescribeImageScanFindingsOutput
	err error
}

func (f *fakeAPI) DescribeImageScanFindings(ctx context.Context, params *awsecr.DescribeImageScanFindingsInput, optFns ...func(*awsecr.Options)) (*awsecr.DescribeImageScanFindingsOutput, error) {
	return f.out, f.err
}

func TestGetScanSummaryIndexed(t *testing.T) {
	api := &fakeAPI{out: &awsecr.DescribeImageScanFindingsOutput{
		ImageScanStatus: &types.ImageScanStatus{Status: types.ScanStatusComplete},
		ImageScanFindings: &types.ImageScanFindings{
			FindingSeverityCounts: map[string]int32{
				"CRITICAL": 1,
				"HIGH":     2,
			},
			Findings: []types.ImageScanFinding{
				{Name: aws.String("CVE-2024-1"), Severity: types.FindingSeverityCritical},
			},
		},
	}}
	c := NewClientWithAPI("eu-west-1", api)
	sum, err := c.GetScanSummary(context.Background(), scan.ArtifactRef{
		ImageRef: "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app:1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Scanned || sum.LookupStatus != scan.StatusIndexed || sum.Critical != 1 || sum.High != 2 {
		t.Fatalf("unexpected summary: %+v", sum)
	}
	if sum.Scanner != scannerName {
		t.Fatalf("scanner=%q", sum.Scanner)
	}
}

func TestGetScanSummaryNotIndexed(t *testing.T) {
	api := &fakeAPI{err: errors.New("ScanNotFoundException: Image scan does not exist")}
	c := NewClientWithAPI("eu-west-1", api)
	sum, err := c.GetScanSummary(context.Background(), scan.ArtifactRef{
		ImageRef: "123456789012.dkr.ecr.eu-west-1.amazonaws.com/app:1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.LookupStatus != scan.StatusNotIndexed {
		t.Fatalf("got %s", sum.LookupStatus)
	}
}
