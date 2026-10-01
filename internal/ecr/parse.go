package ecr

import (
	"strings"

	"github.com/zpol/katana/internal/imageutil"
)

// ImageRef is a parsed private ECR image reference.
type ImageRef struct {
	Registry   string
	AccountID  string
	Region     string
	Repository string
	Tag        string
	Digest     string // without sha256: prefix when possible
}

// IsECRHost reports whether host is a private Amazon ECR registry.
func IsECRHost(host string) bool {
	return imageutil.IsECRHost(host)
}

// ParseImage parses a full image string into ECR coordinates.
func ParseImage(image string) (ImageRef, bool) {
	ref := imageutil.Parse(image)
	account, region, ok := imageutil.ParseECRHost(ref.Registry)
	if !ok {
		return ImageRef{}, false
	}
	digest := strings.TrimPrefix(ref.Digest, "sha256:")
	return ImageRef{
		Registry:   ref.Registry,
		AccountID:  account,
		Region:     region,
		Repository: ref.Name,
		Tag:        ref.Tag,
		Digest:     digest,
	}, true
}
