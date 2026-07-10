package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requiredEnv returns a full set of env vars that satisfies Validate(),
// so individual tests can unset just the one they care about.
func requiredEnv() map[string]string {
	return map[string]string{
		"MPESA_DATABASE_URL":              "postgres://user:pass@localhost:5432/db",
		"MPESA_REDIS_URL":                 "redis://localhost:6379/0",
		"MPESA_SAFARICOM_CONSUMER_KEY":    "key",
		"MPESA_SAFARICOM_CONSUMER_SECRET": "secret",
		"MPESA_SAFARICOM_PASSKEY":         "passkey",
		"MPESA_SAFARICOM_SHORT_CODE":      "174379",
		"MPESA_SAFARICOM_CALLBACK_URL":    "https://example.com/callback",
	}
}

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func TestLoad_AllRequiredVarsPresent(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.ServerPort, "default server port should apply when unset")
	assert.Equal(t, 25, cfg.DBMaxConns)
	assert.Equal(t, 10, cfg.WorkerConcurrency)
}

func TestLoad_MissingRequiredVars(t *testing.T) {
	requiredKeys := []string{
		"MPESA_DATABASE_URL",
		"MPESA_REDIS_URL",
		"MPESA_SAFARICOM_CONSUMER_KEY",
		"MPESA_SAFARICOM_CONSUMER_SECRET",
		"MPESA_SAFARICOM_PASSKEY",
		"MPESA_SAFARICOM_SHORT_CODE",
		"MPESA_SAFARICOM_CALLBACK_URL",
	}

	for _, missing := range requiredKeys {
		t.Run("missing "+missing, func(t *testing.T) {
			env := requiredEnv()
			delete(env, missing)
			setEnv(t, env)
			// Explicitly ensure it's unset even if a prior test run left it
			// in the process environment (t.Setenv already scopes/restores,
			// but being explicit here documents intent).
			t.Setenv(missing, "")

			_, err := Load()

			require.Error(t, err, "expected Load() to fail with %s unset", missing)
		})
	}
}

func TestLoad_SafaricomIPsParsedAndTrimmed(t *testing.T) {
	env := requiredEnv()
	env["MPESA_SAFARICOM_IPS"] = " 196.201.214.200 , 196.201.214.206,196.201.213.114 "
	setEnv(t, env)

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, []string{"196.201.214.200", "196.201.214.206", "196.201.213.114"}, cfg.SafaricomIPs)
}

func TestLoad_EmptySafaricomIPsMeansNoAllowlist(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()

	require.NoError(t, err)
	assert.Empty(t, cfg.SafaricomIPs)
}

func TestLoad_TrustedProxiesParsedAndTrimmed(t *testing.T) {
	env := requiredEnv()
	env["MPESA_TRUSTED_PROXIES"] = " 10.0.0.1 , 10.0.0.0/24 "
	setEnv(t, env)

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.0/24"}, cfg.TrustedProxies)
}

func TestLoad_EmptyTrustedProxiesMeansTrustNone(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()

	require.NoError(t, err)
	assert.Empty(t, cfg.TrustedProxies)
}

func TestLoad_DefaultsToSandboxURLs(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()

	require.NoError(t, err)
	assert.Contains(t, cfg.SafaricomAuthURL, "sandbox.safaricom.co.ke")
	assert.Contains(t, cfg.SafaricomSTKPushURL, "sandbox.safaricom.co.ke")
}
