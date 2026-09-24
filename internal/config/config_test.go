package config

import (
	"testing"
	"time"
)

func TestT06ConfigDefaultsAndBounds(t *testing.T) {
	env := map[string]string{"PERPETUAL_DATABASE_URL": "postgres://fixture"}
	get := func(key string) string { return env[key] }
	c, err := Parse(get)
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	if c.ListenAddress != "127.0.0.1:7777" || c.RuntimeDir != "/run/perpetual" || c.MaxRegistrations != 1024 || c.MaxConnections != 128 || c.MaxJobs != 128 || c.QueueSize != 128 || c.Workers != 4 || c.PoolConnections != 8 || c.LockTimeout != time.Second || c.StatementTimeout != 5*time.Second || c.OperationTimeout != 8*time.Second || c.ShutdownGrace != 10*time.Second {
		t.Fatalf("wrong defaults %+v", c)
	}
	cases := map[string][]string{"PERPETUAL_MAX_REGISTRATIONS": {"0", "-1", "18446744073709551615", "nonsense"}, "PERPETUAL_WORKERS": {"0", "8", "99999999999999"}, "PERPETUAL_POOL_CONNECTIONS": {"0", "4"}, "PERPETUAL_MAX_JOBS": {"0", "-1", "1000000000"}, "PERPETUAL_QUEUE_SIZE": {"0", "-1"}, "PERPETUAL_MAX_CONNECTIONS": {"0", "-1"}, "PERPETUAL_LOCK_TIMEOUT": {"0s", "-1s", "bad"}, "PERPETUAL_STATEMENT_TIMEOUT": {"0s"}, "PERPETUAL_OPERATION_TIMEOUT": {"0s"}, "PERPETUAL_SHUTDOWN_GRACE": {"0s"}, "PERPETUAL_LISTEN_ADDRESS": {"bad-address"}, "PERPETUAL_RUNTIME_DIR": {"relative/path"}}
	for key, values := range cases {
		for _, value := range values {
			env[key] = value
			if _, err := Parse(get); err == nil {
				t.Errorf("accepted %s=%s", key, value)
			}
			delete(env, key)
		}
	}
	delete(env, "PERPETUAL_DATABASE_URL")
	if _, err := Parse(get); err == nil {
		t.Fatal("accepted missing database URL")
	}
}

func TestT06ConfigDecisionPairs(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Error("nil lookup accepted")
	}
	cases := []struct{ key, value string }{{"PERPETUAL_DATABASE_URL", "http://fixture"}, {"PERPETUAL_DATABASE_URL", "postgres:///db"}, {"PERPETUAL_DATABASE_URL", "://bad"}, {"PERPETUAL_LISTEN_ADDRESS", ":7777"}, {"PERPETUAL_LISTEN_ADDRESS", "127.0.0.1:"}, {"PERPETUAL_LISTEN_ADDRESS", "127.0.0.1:0"}, {"PERPETUAL_LISTEN_ADDRESS", "127.0.0.1:65536"}, {"PERPETUAL_LOCK_TIMEOUT", "5s"}, {"PERPETUAL_STATEMENT_TIMEOUT", "8s"}, {"PERPETUAL_MAX_REGISTRATIONS", "1025"}}
	for _, c := range cases {
		t.Run(c.key+c.value, func(t *testing.T) {
			env := map[string]string{"PERPETUAL_DATABASE_URL": "postgres://fixture", c.key: c.value}
			if _, err := Parse(func(k string) string { return env[k] }); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	for _, scheme := range []string{"postgres", "postgresql"} {
		env := map[string]string{"PERPETUAL_DATABASE_URL": scheme + "://fixture", "PERPETUAL_MAX_REGISTRATIONS": "1", "PERPETUAL_WORKERS": "1", "PERPETUAL_POOL_CONNECTIONS": "2", "PERPETUAL_LOCK_TIMEOUT": "500ms"}
		if _, err := Parse(func(k string) string { return env[k] }); err != nil {
			t.Errorf("valid override rejected: %v", err)
		}
	}
}
