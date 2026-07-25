// Copyright (c) 2021 Terminus, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/erda-project/erda/pkg/database/sqllint"
	"github.com/erda-project/erda/pkg/database/sqlparser/migrator"
)

type configuration struct {
	mysql        migrator.DSNParameters
	migrationDir string
	modules      []string
	skipLint     bool
	skipSandbox  bool
	skipPre      bool
	skipMigrate  bool
	debugSQL     bool
	retryTimeout uint64
}

func main() {
	cfg, err := parseConfiguration()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	mig, err := migrator.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create migrator: %v\n", err)
		os.Exit(1)
	}
	if err := mig.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run migrations: %v\n", err)
		os.Exit(1)
	}
}

func parseConfiguration() (*configuration, error) {
	port := envInt("MIGRATION_MYSQL_PORT", 3306)
	retryTimeout := envUint64("MIGRATION_RETRY_TIMEOUT", 150)
	cfg := &configuration{}
	var modules string

	flag.StringVar(&cfg.mysql.Host, "mysql-host", env("MIGRATION_MYSQL_HOST", "127.0.0.1"), "MySQL host")
	flag.IntVar(&port, "mysql-port", port, "MySQL port")
	flag.StringVar(&cfg.mysql.Username, "mysql-username", env("MIGRATION_MYSQL_USERNAME", "root"), "MySQL username")
	flag.StringVar(&cfg.mysql.Password, "mysql-password", os.Getenv("MIGRATION_MYSQL_PASSWORD"), "MySQL password (prefer MIGRATION_MYSQL_PASSWORD)")
	flag.StringVar(&cfg.mysql.Database, "mysql-database", env("MIGRATION_MYSQL_DBNAME", "erda"), "MySQL database")
	flag.StringVar(&cfg.migrationDir, "migration-dir", env("MIGRATION_DIR", ".erda/migrations"), "migration scripts directory")
	flag.StringVar(&modules, "modules", os.Getenv("MIGRATION_MODULES"), "comma-separated migration modules")
	flag.BoolVar(&cfg.skipLint, "skip-lint", envBool("MIGRATION_SKIP_LINT", true), "skip SQL lint")
	flag.BoolVar(&cfg.skipSandbox, "skip-sandbox", envBool("MIGRATION_SKIP_SANDBOX", true), "skip sandbox migration")
	flag.BoolVar(&cfg.skipPre, "skip-pre-migration", envBool("MIGRATION_SKIP_PRE_MIGRATION", true), "skip pre-migration")
	flag.BoolVar(&cfg.skipMigrate, "skip-migration", envBool("MIGRATION_SKIP_MIGRATION", false), "validate without applying migrations")
	flag.BoolVar(&cfg.debugSQL, "debug-sql", envBool("MIGRATION_DEBUGSQL", false), "log executed SQL")
	flag.Parse()

	if cfg.mysql.Host == "" || cfg.mysql.Username == "" || cfg.mysql.Database == "" || cfg.migrationDir == "" {
		return nil, fmt.Errorf("mysql host, username, database and migration directory are required")
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid MySQL port %d", port)
	}
	cfg.mysql.Port = port
	cfg.mysql.ParseTime = true
	cfg.mysql.Timeout = time.Duration(retryTimeout) * time.Second
	cfg.retryTimeout = retryTimeout
	for _, module := range strings.Split(modules, ",") {
		if module = strings.TrimSpace(module); module != "" {
			cfg.modules = append(cfg.modules, module)
		}
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func envUint64(key string, fallback uint64) uint64 {
	value, err := strconv.ParseUint(os.Getenv(key), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func (c *configuration) Workdir() string { return "" }

func (c *configuration) MigrationDir() string { return c.migrationDir }

func (c *configuration) Modules() []string { return c.modules }

func (c *configuration) LintConfig() map[string]sqllint.Config { return nil }

func (c *configuration) MySQLParameters() *migrator.DSNParameters { return &c.mysql }

func (c *configuration) SandboxParameters() *migrator.DSNParameters { return &c.mysql }

func (c *configuration) DebugSQL() bool { return c.debugSQL }

func (c *configuration) RetryTimeout() uint64 { return c.retryTimeout }

func (c *configuration) SkipMigrationLint() bool { return c.skipLint }

func (c *configuration) SkipSandbox() bool { return c.skipSandbox }

func (c *configuration) SkipPreMigrate() bool { return c.skipPre }

func (c *configuration) SkipMigrate() bool { return c.skipMigrate }
