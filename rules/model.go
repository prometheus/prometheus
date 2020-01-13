package rules

import (
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/go-xorm/xorm"
	"github.com/prometheus/prometheus/config"
)

var (
	Engine *xorm.Engine
)

func NewEngine(c config.MysqlRuleConfig) {
	var err error
	str := fmt.Sprintf("%v:%v@(%v:%v)/%v?charset=utf8", c.User, c.Passwd, c.Host, c.Port, c.Db)

	Engine, err = xorm.NewEngine("mysql", str)

	if err != nil {
		return
	}

	go func() {
		for {
			select {
			case <-time.After(time.Minute * time.Duration(5)):
				Engine.Ping()
			}
		}
	}()
}

type AlertRule struct {
	Id         int `xorm:"pk autoincr int(11)"`
	Name       string
	Expr       string
	Severity   string
	Descd      string
	Inter      string
	CreateTime time.Time `DateTime xorm:"created"`
	Status     bool
	ForInter   string
}

type RuleHistory struct {
	Id         int `xorm:"pk autoincr int(11)"`
	Name       string
	Expr       string
	Severity   string
	Descd      string
	Inter      string
	CreateTime time.Time `DateTime xorm:"created"`
	Optype     string
	ForInter   string
}

func NewHistory(a *AlertRule, opType string) {
	r := &RuleHistory{}
	r.Name = a.Name
	r.Expr = a.Expr
	r.Severity = a.Severity
	r.Descd = a.Descd
	r.Inter = a.Inter
	r.CreateTime = time.Now()
	r.Optype = opType
	r.ForInter = a.ForInter
	Engine.Insert(r)
}

var sql = `CREATE TABLE alert_rule  ( 
    id   INT PRIMARY KEY AUTO_INCREMENT, 
    name VARCHAR (600) NOT NULL,
    expr text NOT NULL,
    severity VARCHAR (500) NOT NULL,
    descd TEXT NOT NULL,
    inter VARCHAR (500) NOT NULL,
    create_time datetime DEFAULT NULL,
    status tinyint(1) DEFAULT 1
);`

var sqlHistory = `CREATE TABLE rule_history  ( 
    id   INT PRIMARY KEY AUTO_INCREMENT, 
    name VARCHAR (600) NOT NULL,
    expr text NOT NULL,
    severity VARCHAR (500) NOT NULL,
    descd TEXT NOT NULL,
    inter VARCHAR (500) NOT NULL,
    create_time datetime DEFAULT NULL,
    status tinyint(1) DEFAULT 1,
    optype VARCHAR (600) NOT NULL
);`
