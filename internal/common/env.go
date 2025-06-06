package common

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yusing/go-proxy/internal/utils/strutils"
)

var (
	prefixes = []string{"GODOXY_", "GOPROXY_", ""}

	IsTest  = GetEnvBool("TEST", false) || strings.HasSuffix(os.Args[0], ".test")
	IsDebug = GetEnvBool("DEBUG", IsTest)
	IsTrace = GetEnvBool("TRACE", false) && IsDebug

	HTTP3Enabled = GetEnvBool("HTTP3_ENABLED", true)

	ProxyHTTPAddr,
	ProxyHTTPHost,
	ProxyHTTPPort,
	ProxyHTTPURL = GetAddrEnv("HTTP_ADDR", ":80", "http")

	ProxyHTTPSAddr,
	ProxyHTTPSHost,
	ProxyHTTPSPort,
	ProxyHTTPSURL = GetAddrEnv("HTTPS_ADDR", ":443", "https")

	APIHTTPAddr,
	APIHTTPHost,
	APIHTTPPort,
	APIHTTPURL = GetAddrEnv("API_ADDR", "127.0.0.1:8888", "http")

	APIJWTSecure   = GetEnvBool("API_JWT_SECURE", true)
	APIJWTSecret   = decodeJWTKey(GetEnvString("API_JWT_SECRET", ""))
	APIJWTTokenTTL = GetDurationEnv("API_JWT_TOKEN_TTL", 24*time.Hour)
	APIUser        = GetEnvString("API_USER", "admin")
	APIPassword    = GetEnvString("API_PASSWORD", "password")

	DebugDisableAuth = GetEnvBool("DEBUG_DISABLE_AUTH", false)

	// OIDC Configuration.
	OIDCIssuerURL     = GetEnvString("OIDC_ISSUER_URL", "")
	OIDCClientID      = GetEnvString("OIDC_CLIENT_ID", "")
	OIDCClientSecret  = GetEnvString("OIDC_CLIENT_SECRET", "")
	OIDCScopes        = GetCommaSepEnv("OIDC_SCOPES", "openid, profile, email, groups")
	OIDCAllowedUsers  = GetCommaSepEnv("OIDC_ALLOWED_USERS", "")
	OIDCAllowedGroups = GetCommaSepEnv("OIDC_ALLOWED_GROUPS", "")

	// metrics configuration
	MetricsDisableCPU     = GetEnvBool("METRICS_DISABLE_CPU", false)
	MetricsDisableMemory  = GetEnvBool("METRICS_DISABLE_MEMORY", false)
	MetricsDisableDisk    = GetEnvBool("METRICS_DISABLE_DISK", false)
	MetricsDisableNetwork = GetEnvBool("METRICS_DISABLE_NETWORK", false)
	MetricsDisableSensors = GetEnvBool("METRICS_DISABLE_SENSORS", false)

	ForceResolveCountry = GetEnvBool("FORCE_RESOLVE_COUNTRY", false)
)

func GetEnv[T any](key string, defaultValue T, parser func(string) (T, error)) T {
	var value string
	var ok bool
	for _, prefix := range prefixes {
		value, ok = os.LookupEnv(prefix + key)
		if ok && value != "" {
			break
		}
	}
	if !ok || value == "" {
		return defaultValue
	}
	parsed, err := parser(value)
	if err == nil {
		return parsed
	}
	log.Fatalf("env %s: invalid %T value: %s", key, parsed, value)
	return defaultValue
}

func stringstring(s string) (string, error) {
	return s, nil
}

func GetEnvString(key string, defaultValue string) string {
	return GetEnv(key, defaultValue, stringstring)
}

func GetEnvBool(key string, defaultValue bool) bool {
	return GetEnv(key, defaultValue, strconv.ParseBool)
}

func GetEnvInt(key string, defaultValue int) int {
	return GetEnv(key, defaultValue, strconv.Atoi)
}

func GetAddrEnv(key, defaultValue, scheme string) (addr, host string, portInt int, fullURL string) {
	addr = GetEnvString(key, defaultValue)
	if addr == "" {
		return
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		log.Fatalf("env %s: invalid address: %s", key, addr)
	}
	if host == "" {
		host = "localhost"
	}
	fullURL = fmt.Sprintf("%s://%s:%s", scheme, host, port)
	portInt, err = strconv.Atoi(port)
	if err != nil {
		log.Fatalf("env %s: invalid port: %s", key, port)
	}
	return
}

func GetDurationEnv(key string, defaultValue time.Duration) time.Duration {
	return GetEnv(key, defaultValue, time.ParseDuration)
}

func GetCommaSepEnv(key string, defaultValue string) []string {
	return strutils.CommaSeperatedList(GetEnvString(key, defaultValue))
}
