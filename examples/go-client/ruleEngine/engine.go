package ruleEngine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Comcast/rulio/core"
	"github.com/Comcast/rulio/cron"
	"github.com/Comcast/rulio/examples/go-client/configuration"
	"github.com/Comcast/rulio/examples/go-client/service"
	"github.com/Comcast/rulio/sys"
	"os/exec"
	"strings"
	"time"
)

type EnginePoc struct {
	Ctx     *core.Context
	Service *service.Service
}

func NewEngine(envConfig *configuration.AppEnvConfigPoc, ctx *core.Context) (*EnginePoc, error) {

	inMemoryConf := sys.ExampleConfig()
	if envConfig.LinearState {
		inMemoryConf.UnindexedState = true
	}
	inMemoryConf.Storage = envConfig.StorageType
	inMemoryConf.StorageConfig = envConfig.StorageConfig

	cont := sys.ExampleSystemControl()
	cont.MaxLocations = envConfig.MaxLocations
	ttl, err := sys.ParseLocationTTL(envConfig.LocationTTL)
	if err != nil {
		return nil, err
	}
	cont.LocationTTL = ttl

	var cronner cron.Cronner
	if envConfig.CronURL == "" {
		cr, _ := cron.NewCron(nil, time.Second, "intcron", 1000000)
		go cr.Start(ctx)
		cronner = &cron.InternalCron{Cron: cr}
	} else {
		cronner = &cron.CroltSimple{
			CroltURL: envConfig.CronURL,
			RulesURL: envConfig.RulesURL,
		}
	}

	verb, err := core.ParseVerbosity(envConfig.Verbosity)
	if err != nil {
		panic(err)
	}
	locCtl := &core.Control{
		MaxFacts:  envConfig.MaxFacts,
		Verbosity: verb,
	}
	if envConfig.BashActions {
		locCtl.ActionInterpreters = map[string]core.ActionInterpreter{
			"bash": &BashActionInterpreter{},
		}
	}
	cont.DefaultLocControl = locCtl

	// TODO: New system is being created
	sys, err := sys.NewSystem(ctx, *inMemoryConf, *cont, cronner)
	if err != nil {
		return nil, err
	}
	ctx.SetLogValue("app.id", "rulesys")
	core.UseCores(ctx, false)
	ctx.Verbosity = verb
	ctx.LogAccumulatorLevel = verb

	engineService := &service.Service{
		System:  sys,
		Router:  nil,
		Stopper: nil,
	}
	return &EnginePoc{Ctx: ctx, Service: engineService}, nil
}

type BashActionInterpreter struct {
}

func (i *BashActionInterpreter) GetName() string {
	return "bash"
}

func (i *BashActionInterpreter) GetThunk(ctx *core.Context, loc *core.Location, bs core.Bindings, a core.Action) (func() (interface{}, error), error) {

	return func() (interface{}, error) {
		core.Log(core.DEBUG, ctx, "BashActionIntpreter.GetThunk", "action", a, "bs", bs)
		code, err := core.GetCode(a.Code)
		if err != nil {
			return nil, err
		}

		env := make([]string, 0, len(bs))
		for p, v := range bs {
			p = strings.ToUpper(p[1:])
			if p == "EVENT" {
				js, err := json.Marshal(&v)
				if err != nil {
					return nil, err
				}
				v = string(js)
			}
			env = append(env, fmt.Sprintf("_%s=%s", p, v))
		}

		cmd := exec.Command("bash")
		cmd.Stdin = strings.NewReader(code)
		cmd.Env = env
		var out bytes.Buffer
		cmd.Stdout = &out
		if err = cmd.Run(); err != nil {
			core.Log(core.INFO, ctx, "BashActionIntpreter.GetThunk", "action", a, "error", err)
			return nil, err
		}
		return out.String(), nil
	}, nil

}
