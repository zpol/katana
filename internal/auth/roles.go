package auth

const (
	RoleAdmin    = "admin"
	RoleReadonly = "readonly"
)

func ValidRole(r string) bool {
	return r == RoleAdmin || r == RoleReadonly
}
