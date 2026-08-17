package watchadmin

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benenen/lapin/internal/config"
)

const (
	DefaultEmail    = "admin@localhost"
	DefaultPassword = "admin12345678"
)

func Validate(settings config.Config) error {
	if settings.AdminEmail != DefaultEmail || settings.AdminPassword != DefaultPassword {
		return nil
	}
	if !strings.EqualFold(settings.Environment, "development") {
		return fmt.Errorf("default watch administrator is only allowed in development")
	}
	if !validLoopbackHTTPAddress(settings.HTTPAddress) {
		return fmt.Errorf("default watch administrator requires a loopback HTTP_ADDR")
	}
	poolConfig, err := pgxpool.ParseConfig(settings.DatabaseURL)
	if err != nil {
		return fmt.Errorf("default watch administrator requires a valid loopback PostgreSQL DATABASE_URL")
	}
	if !loopbackHost(poolConfig.ConnConfig.Host) {
		return fmt.Errorf("default watch administrator requires a loopback PostgreSQL DATABASE_URL")
	}
	for _, fallback := range poolConfig.ConnConfig.Fallbacks {
		if !loopbackHost(fallback.Host) {
			return fmt.Errorf("default watch administrator requires a loopback PostgreSQL DATABASE_URL")
		}
	}
	return nil
}

func validLoopbackHTTPAddress(address string) bool {
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil || !loopbackHost(host) {
		return false
	}
	port, err := strconv.ParseUint(rawPort, 10, 16)
	return err == nil && port > 0
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
