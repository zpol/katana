package ecr

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awsecr "github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/zpol/katana/internal/metrics"
	"github.com/zpol/katana/internal/scan"
)

const scannerName = "ecr_inspector"

// API is the ECR subset used for scan findings (mockable in tests).
type API interface {
	DescribeImageScanFindings(ctx context.Context, params *awsecr.DescribeImageScanFindingsInput, optFns ...func(*awsecr.Options)) (*awsecr.DescribeImageScanFindingsOutput, error)
}

// Client talks to Amazon ECR / Inspector enhanced scan findings.
type Client struct {
	region string
	api    API
	mu     sync.Mutex
	cache  map[string]cacheEntry
}

type cacheEntry struct {
	at  time.Time
	sum *scan.Summary
}

// NewClient builds an ECR scanner using the default AWS credential chain (IRSA / env / shared config).
// Empty region uses AWS_REGION / AWS_DEFAULT_REGION from the environment (resolved per-call from the image when possible).
func NewClient(region string) *Client {
	return &Client{
		region: strings.TrimSpace(region),
		cache:  map[string]cacheEntry{},
	}
}

// NewClientWithAPI injects a custom ECR API (tests).
func NewClientWithAPI(region string, api API) *Client {
	c := NewClient(region)
	c.api = api
	return c
}

// Name implements scan.Client.
func (c *Client) Name() string { return scannerName }

// Configured reports whether AWS credentials/region are usable enough to attempt lookups.
// We treat presence of region (env or configured) OR any AWS cred env / IRSA as configured;
// actual reachability is checked in Status.
func (c *Client) Configured() bool {
	if c == nil {
		return false
	}
	if c.api != nil {
		return true
	}
	if c.region != "" || os.Getenv("AWS_REGION") != "" || os.Getenv("AWS_DEFAULT_REGION") != "" {
		return true
	}
	// IRSA / classic keys
	if os.Getenv("AWS_ROLE_ARN") != "" || os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" {
		return true
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" {
		return true
	}
	return false
}

// Status probes AWS by loading default config (no ECR round-trip required for "configured").
func (c *Client) Status(ctx context.Context) scan.Status {
	if c == nil || !c.Configured() {
		return scan.Status{
			Configured: false,
			Scanner:    scannerName,
			Message:    "AWS region/credentials not set (use IRSA or AWS_REGION + credentials)",
		}
	}
	st := scan.Status{Configured: true, Scanner: scannerName, BaseURL: "ecr://" + c.effectiveRegion("")}
	api, err := c.apiFor(ctx, c.effectiveRegion(""))
	if err != nil {
		st.Reachable = false
		st.Message = err.Error()
		return st
	}
	_ = api
	st.Reachable = true
	st.Message = "ok (credential chain loaded)"
	return st
}

func (c *Client) effectiveRegion(fromImage string) string {
	if fromImage != "" {
		return fromImage
	}
	if c.region != "" {
		return c.region
	}
	if v := strings.TrimSpace(os.Getenv("AWS_REGION")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
}

func (c *Client) apiFor(ctx context.Context, region string) (API, error) {
	if c.api != nil {
		return c.api, nil
	}
	opts := []func(*config.LoadOptions) error{}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	return awsecr.NewFromConfig(cfg), nil
}

// GetScanSummary loads ECR image scan findings (enhanced/basic via DescribeImageScanFindings).
func (c *Client) GetScanSummary(ctx context.Context, ref scan.ArtifactRef) (sum *scan.Summary, err error) {
	start := time.Now()
	defer func() {
		if sum != nil && sum.Scanner == "" {
			sum.Scanner = scannerName
		}
		status := scan.StatusUnavailable
		if sum != nil && sum.LookupStatus != "" {
			status = sum.LookupStatus
		}
		metrics.ObserveScanLookup(scannerName, status, time.Since(start))
	}()

	if c == nil || !c.Configured() {
		return scan.Unavailable(ref, scannerName, "ecr scanner not configured"), nil
	}

	ecrRef, ok := ParseImage(ref.ImageRef)
	if !ok {
		return scan.Unavailable(ref, scannerName, "image is not a private ECR registry reference"), nil
	}
	if ref.Sha != "" {
		ecrRef.Digest = strings.TrimPrefix(ref.Sha, "sha256:")
	}

	if cached := c.cacheGet(ref); cached != nil {
		return cached, nil
	}

	api, err := c.apiFor(ctx, ecrRef.Region)
	if err != nil {
		return scan.Unavailable(ref, scannerName, err.Error()), nil
	}

	input := &awsecr.DescribeImageScanFindingsInput{
		RepositoryName: aws.String(ecrRef.Repository),
		ImageId:        &types.ImageIdentifier{},
	}
	if ecrRef.Digest != "" {
		input.ImageId.ImageDigest = aws.String("sha256:" + strings.TrimPrefix(ecrRef.Digest, "sha256:"))
	} else if ecrRef.Tag != "" {
		input.ImageId.ImageTag = aws.String(ecrRef.Tag)
	} else {
		return scan.Unavailable(ref, scannerName, "need image tag or digest for ECR lookup"), nil
	}
	if ecrRef.AccountID != "" {
		input.RegistryId = aws.String(ecrRef.AccountID)
	}

	out, err := api.DescribeImageScanFindings(ctx, input)
	if err != nil {
		msg := err.Error()
		low := strings.ToLower(msg)
		if strings.Contains(low, "scannotfound") || strings.Contains(low, "scan not found") ||
			strings.Contains(low, "imagenotfoundexception") || strings.Contains(low, "does not have a completed scan") {
			sum = scan.NotIndexed(ref, scannerName, "no completed ECR/Inspector scan for this image")
			c.cachePut(ref, sum)
			return sum, nil
		}
		return scan.Unavailable(ref, scannerName, msg), nil
	}

	sum = mapFindings(ref, out)
	if sum.LookupStatus == scan.StatusIndexed || sum.LookupStatus == scan.StatusNotIndexed {
		c.cachePut(ref, sum)
	}
	return sum, nil
}

func mapFindings(ref scan.ArtifactRef, out *awsecr.DescribeImageScanFindingsOutput) *scan.Summary {
	sum := &scan.Summary{
		Artifact:     ref,
		Scanner:      scannerName,
		LookupStatus: scan.StatusNotIndexed,
		Scanned:      false,
	}
	if out == nil {
		sum.RawError = "empty DescribeImageScanFindings response"
		return sum
	}
	if out.ImageScanStatus != nil {
		switch out.ImageScanStatus.Status {
		case types.ScanStatusFailed, types.ScanStatusFindingsUnavailable, types.ScanStatusLimitExceeded:
			return scan.Unavailable(ref, scannerName, aws.ToString(out.ImageScanStatus.Description))
		case types.ScanStatusInProgress, types.ScanStatusPending:
			return scan.Unavailable(ref, scannerName, "ECR scan still in progress (not waiting in admission)")
		case types.ScanStatusUnsupportedImage, types.ScanStatusImageArchived:
			return scan.NotIndexed(ref, scannerName, "ECR scan unsupported for this image type")
		}
	}
	findings := out.ImageScanFindings
	if findings == nil {
		sum.RawError = "no scan findings payload"
		return sum
	}

	sum.Scanned = true
	sum.LookupStatus = scan.StatusIndexed

	if findings.FindingSeverityCounts != nil {
		for sev, n := range findings.FindingSeverityCounts {
			switch strings.ToUpper(sev) {
			case "CRITICAL":
				sum.Critical += int(n)
			case "HIGH":
				sum.High += int(n)
			case "MEDIUM":
				sum.Medium += int(n)
			case "LOW":
				sum.Low += int(n)
			default:
				sum.Unknown += int(n)
			}
		}
	}

	// Prefer enhanced findings titles when present for violation samples.
	for _, ef := range findings.EnhancedFindings {
		if len(sum.Violations) >= 25 {
			break
		}
		title := aws.ToString(ef.Title)
		if title == "" {
			title = aws.ToString(ef.Description)
		}
		sev := aws.ToString(ef.Severity)
		if title != "" {
			sum.Violations = append(sum.Violations, fmt.Sprintf("%s (%s)", title, strings.ToLower(sev)))
		}
	}
	if len(sum.Violations) == 0 {
		for _, f := range findings.Findings {
			if len(sum.Violations) >= 25 {
				break
			}
			name := aws.ToString(f.Name)
			sev := string(f.Severity)
			if name != "" {
				sum.Violations = append(sum.Violations, fmt.Sprintf("%s (%s)", name, strings.ToLower(sev)))
			}
		}
	}
	return sum
}

func (c *Client) cacheGet(ref scan.ArtifactRef) *scan.Summary {
	key := cacheKey(ref)
	if key == "|" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ent, ok := c.cache[key]
	if !ok || time.Since(ent.at) > 5*time.Minute {
		return nil
	}
	return ent.sum
}

func (c *Client) cachePut(ref scan.ArtifactRef, sum *scan.Summary) {
	if sum == nil || sum.LookupStatus == scan.StatusUnavailable {
		return
	}
	key := cacheKey(ref)
	if key == "|" {
		return
	}
	c.mu.Lock()
	c.cache[key] = cacheEntry{at: time.Now(), sum: sum}
	c.mu.Unlock()
}

func cacheKey(ref scan.ArtifactRef) string {
	return strings.TrimSpace(ref.ImageRef) + "|" + strings.TrimSpace(ref.Sha)
}
