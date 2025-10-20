package conf

import (
	"context"
	"database/sql"
	entsql "entgo.io/ent/dialect/sql"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"time"
)

// DatabaseConf stores database configurations.
type DatabaseConf struct {
	Host         string `json:",env=DATABASE_HOST"`
	Port         int    `json:",env=DATABASE_PORT"`
	Username     string `json:",default=root,env=DATABASE_USERNAME"`
	Password     string `json:",optional,env=DATABASE_PASSWORD"`
	DBName       string `json:",default=simple_admin,env=DATABASE_DBNAME"`
	SSLMode      string `json:",optional,env=DATABASE_SSL_MODE"`
	Type         string `json:",default=mysql,options=[mysql,postgres,sqlite3],env=DATABASE_TYPE"`
	MaxOpenConn  int    `json:",optional,default=100,env=DATABASE_MAX_OPEN_CONN"`
	CacheTime    int    `json:",optional,default=10,env=DATABASE_CACHE_TIME"`
	DBPath       string `json:",optional,env=DATABASE_DBPATH"`
	MysqlConfig  string `json:",optional,env=DATABASE_MYSQL_CONFIG"`
	PGConfig     string `json:",optional,env=DATABASE_PG_CONFIG"`
	SqliteConfig string `json:",optional,env=DATABASE_SQLITE_CONFIG"`
}

var (
	// ExitOnFatal defines whether to exit on fatal errors, defined here to make it easier to test.
	ExitOnFatal = true
)

func LogMust(err error) {
	if err == nil {
		return
	}

	logrus.Error(err)
	if ExitOnFatal {
		os.Exit(1)
	} else {
		panic(err)
	}
}

// NewNoCacheDriver returns an Ent driver without cache.
func (c DatabaseConf) NewNoCacheDriver() *entsql.Driver {
	db, err := sql.Open(c.Type, c.GetDSN())
	LogMust(err)

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	err = db.PingContext(ctx)
	LogMust(err)

	db.SetMaxOpenConns(c.MaxOpenConn)
	driver := entsql.OpenDB(c.Type, db)

	return driver
}

// MysqlDSN returns mysql DSN.
func (c DatabaseConf) MysqlDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=True%s", c.Username, c.Password, c.Host, c.Port, c.DBName, c.MysqlConfig)
}

// PostgresDSN returns Postgres DSN.
func (c DatabaseConf) PostgresDSN() string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s%s", c.Username, c.Password, c.Host, c.Port, c.DBName,
		c.SSLMode, c.PGConfig)
}

// SqliteDSN returns Sqlite DSN.
func (c DatabaseConf) SqliteDSN() string {
	if c.DBPath == "" {
		LogMust(errors.New("the database file path cannot be empty"))
	}

	if _, err := os.Stat(c.DBPath); os.IsNotExist(err) {
		f, err := os.OpenFile(c.DBPath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			LogMust(fmt.Errorf("failed to create SQLite database file %q", c.DBPath))
		}
		if err := f.Close(); err != nil {
			LogMust(fmt.Errorf("failed to create SQLite database file %q", c.DBPath))
		}
	} else {
		if err := os.Chmod(c.DBPath, 0660); err != nil {
			LogMust(fmt.Errorf("unable to set permission code on %s: %v", c.DBPath, err))
		}
	}

	return fmt.Sprintf("file:%s?_busy_timeout=100000&_fk=1%s", c.DBPath, c.SqliteConfig)
}

// GetDSN returns DSN according to the database type.
func (c DatabaseConf) GetDSN() string {
	switch c.Type {
	case "mysql":
		return c.MysqlDSN()
	case "postgres":
		return c.PostgresDSN()
	case "sqlite3":
		return c.SqliteDSN()
	default:
		return "mysql"
	}
}
