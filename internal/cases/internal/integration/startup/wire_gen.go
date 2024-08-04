// Code generated by Wire. DO NOT EDIT.

//go:generate go run -mod=mod github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package startup

import (
	"github.com/ecodeclub/webook/internal/cases"
	"github.com/ecodeclub/webook/internal/cases/internal/event"
	"github.com/ecodeclub/webook/internal/cases/internal/repository"
	"github.com/ecodeclub/webook/internal/cases/internal/repository/dao"
	"github.com/ecodeclub/webook/internal/cases/internal/service"
	"github.com/ecodeclub/webook/internal/cases/internal/web"
	"github.com/ecodeclub/webook/internal/interactive"
	"github.com/ecodeclub/webook/internal/test/ioc"
)

// Injectors from wire.go:

func InitModule(syncProducer event.SyncEventProducer, intrModule *interactive.Module) (*cases.Module, error) {
	db := testioc.InitDB()
	caseDAO := cases.InitCaseDAO(db)
	caseRepo := repository.NewCaseRepo(caseDAO)
	mq := testioc.InitMQ()
	interactiveEventProducer, err := event.NewInteractiveEventProducer(mq)
	if err != nil {
		return nil, err
	}
	serviceService := service.NewService(caseRepo, interactiveEventProducer, syncProducer)
	service2 := intrModule.Svc
	handler := web.NewHandler(serviceService, service2)
	caseSetDAO := dao.NewCaseSetDAO(db)
	caseSetRepository := repository.NewCaseSetRepo(caseSetDAO)
	caseSetService := service.NewCaseSetService(caseSetRepository)
	adminCaseSetHandler := web.NewAdminCaseSetHandler(caseSetService)
	module := &cases.Module{
		Svc:             serviceService,
		Hdl:             handler,
		AdminSetHandler: adminCaseSetHandler,
	}
	return module, nil
}
