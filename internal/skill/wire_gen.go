// Code generated by Wire. DO NOT EDIT.

//go:generate go run -mod=mod github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package skill

import (
	"github.com/ecodeclub/ecache"
	"github.com/ecodeclub/mq-api"
	"github.com/ecodeclub/webook/internal/cases"
	"github.com/ecodeclub/webook/internal/question"
	"github.com/ecodeclub/webook/internal/skill/internal/event"
	"github.com/ecodeclub/webook/internal/skill/internal/repository"
	"github.com/ecodeclub/webook/internal/skill/internal/repository/cache"
	"github.com/ecodeclub/webook/internal/skill/internal/repository/dao"
	"github.com/ecodeclub/webook/internal/skill/internal/service"
	"github.com/ecodeclub/webook/internal/skill/internal/web"
	"github.com/ego-component/egorm"
	"gorm.io/gorm"
	"sync"
)

// Injectors from wire.go:

func InitHandler(db *gorm.DB, ec ecache.Cache, queModule *baguwen.Module, caseModule *cases.Module, q mq.MQ) (*web.Handler, error) {
	skillDAO := InitSkillDAO(db)
	skillCache := cache.NewSkillCache(ec)
	skillRepo := repository.NewSkillRepo(skillDAO, skillCache)
	syncEventProducer := initSyncEventProducer(q)
	skillService := service.NewSkillService(skillRepo, syncEventProducer)
	serviceService := queModule.Svc
	service2 := caseModule.Svc
	handler := web.NewHandler(skillService, serviceService, service2)
	return handler, nil
}

// wire.go:

var daoOnce = sync.Once{}

func InitTableOnce(db *gorm.DB) {
	daoOnce.Do(func() {
		err := dao.InitTables(db)
		if err != nil {
			panic(err)
		}
	})
}

func InitSkillDAO(db *egorm.Component) dao.SkillDAO {
	InitTableOnce(db)
	return dao.NewSkillDAO(db)
}

func initSyncEventProducer(q mq.MQ) event.SyncEventProducer {
	producer, err := event.NewSyncEventProducer(q)
	if err != nil {
		panic(err)
	}
	return producer
}

type Handler = web.Handler
