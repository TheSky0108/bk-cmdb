/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.
 * Copyright (C) 2017-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package service

import (
	"context"

	"configcenter/src/auth"
	"configcenter/src/common"
	"configcenter/src/common/backbone"
	"configcenter/src/common/errors"
	"configcenter/src/common/metadata"
	"configcenter/src/common/metric"
	"configcenter/src/common/rdapi"
	"configcenter/src/common/types"
	"configcenter/src/storage/dal"

	"github.com/emicklei/go-restful"
	redis "gopkg.in/redis.v5"
)

type Service struct {
	*backbone.Engine
	db    dal.RDB
	cache *redis.Client
	auth  auth.Authorize
	ctx   context.Context
}

func NewService(ctx context.Context) *Service {
	return &Service{
		ctx: ctx,
	}
}

func (s *Service) SetDB(db dal.RDB) {
	s.db = db
}

func (s *Service) SetCache(db *redis.Client) {
	s.cache = db
}

func (s *Service) SetAuth(auth auth.Authorize) {
	s.auth = auth
}

func (s *Service) WebService() *restful.Container {

	container := restful.NewContainer()

	api := new(restful.WebService)
	getErrFunc := func() errors.CCErrorIf {
		return s.CCErr
	}
	api.Path("/event/v3").Filter(s.Engine.Metric().RestfulMiddleWare).Filter(rdapi.AllGlobalFilter(getErrFunc)).Produces(restful.MIME_JSON)

	api.Route(api.POST("/subscribe/search/{ownerID}/{appID}").To(s.Query))
	api.Route(api.POST("/subscribe/ping").To(s.Ping))
	api.Route(api.POST("/subscribe/telnet").To(s.Telnet))
	api.Route(api.POST("/subscribe/{ownerID}/{appID}").To(s.Subscribe))
	api.Route(api.DELETE("/subscribe/{ownerID}/{appID}/{subscribeID}").To(s.UnSubscribe))
	api.Route(api.PUT("/subscribe/{ownerID}/{appID}/{subscribeID}").To(s.Rebook))

	container.Add(api)

	healthzAPI := new(restful.WebService).Produces(restful.MIME_JSON)
	healthzAPI.Route(healthzAPI.GET("/healthz").To(s.Healthz))
	container.Add(healthzAPI)

	return container
}

func (s *Service) Healthz(req *restful.Request, resp *restful.Response) {
	meta := metric.HealthMeta{IsHealthy: true}

	// zk health status
	zkItem := metric.HealthItem{IsHealthy: true, Name: types.CCFunctionalityServicediscover}
	if err := s.Engine.Ping(); err != nil {
		zkItem.IsHealthy = false
		zkItem.Message = err.Error()
	}
	meta.Items = append(meta.Items, zkItem)

	// mongodb
	meta.Items = append(meta.Items, metric.NewHealthItem(types.CCFunctionalityMongo, s.db.Ping()))

	// redis
	meta.Items = append(meta.Items, metric.NewHealthItem(types.CCFunctionalityRedis, s.cache.Ping().Err()))

	for _, item := range meta.Items {
		if item.IsHealthy == false {
			meta.IsHealthy = false
			meta.Message = "event server is unhealthy"
			break
		}
	}

	info := metric.HealthInfo{
		Module:     types.CC_MODULE_EVENTSERVER,
		HealthMeta: meta,
		AtTime:     metadata.Now(),
	}

	answer := metric.HealthResponse{
		Code:    common.CCSuccess,
		Data:    info,
		OK:      meta.IsHealthy,
		Result:  meta.IsHealthy,
		Message: meta.Message,
	}
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteEntity(answer)
}
