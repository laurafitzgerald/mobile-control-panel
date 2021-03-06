/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apiserver

import (
	"github.com/feedhenry/mobile-control-panel/server/pkg/apis/mobile"
	"github.com/feedhenry/mobile-control-panel/server/pkg/apis/mobile/install"
	"github.com/feedhenry/mobile-control-panel/server/pkg/apis/mobile/v1alpha1"
	clientset "github.com/feedhenry/mobile-control-panel/server/pkg/client/clientset_generated/internalclientset"
	"github.com/feedhenry/mobile-control-panel/server/pkg/mobile/broker"
	"github.com/feedhenry/mobile-control-panel/server/pkg/mobile/broker/operations"
	mobilestorage "github.com/feedhenry/mobile-control-panel/server/pkg/registry/mobileapp"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apimachinery/announced"
	"k8s.io/apimachinery/pkg/apimachinery/registered"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
)

var (
	groupFactoryRegistry = make(announced.APIGroupFactoryRegistry)
	registry             = registered.NewOrDie("")
	Scheme               = runtime.NewScheme()
	Codecs               = serializer.NewCodecFactory(Scheme)
)

func init() {
	install.Install(groupFactoryRegistry, registry, Scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

type Config struct {
	GenericConfig *genericapiserver.Config
}

// WardleServer contains state for a Kubernetes cluster master/api server.
type MobileServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	*Config
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (c *Config) Complete() completedConfig {
	c.GenericConfig.Complete()

	c.GenericConfig.Version = &version.Info{
		Major: "1",
		Minor: "0",
	}

	return completedConfig{c}
}

// SkipComplete provides a way to construct a server instance without config completion.
func (c *Config) SkipComplete() completedConfig {
	return completedConfig{c}
}

// New returns a new instance of MobileServer from the given config.
func (c completedConfig) New() (*MobileServer, error) {
	genericServer, err := c.Config.GenericConfig.SkipComplete().New() // completion is done in Complete, no need for a second time
	if err != nil {
		return nil, errors.Wrap(err, "failed get GenericConfig")
	}

	s := &MobileServer{
		GenericAPIServer: genericServer,
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(mobile.GroupName, registry, Scheme, metav1.ParameterCodec, Codecs)
	apiGroupInfo.GroupMeta.GroupVersion = v1alpha1.SchemeGroupVersion
	v1alpha1storage := map[string]rest.Storage{}
	v1alpha1storage[mobile.MobileAppsResource] = mobilestorage.NewREST(Scheme, c.GenericConfig.RESTOptionsGetter)
	apiGroupInfo.VersionedResourcesStorageMap[v1alpha1.APIGroupVersion] = v1alpha1storage

	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, errors.Wrap(err, "failed get InstallAPIGroup")
	}
	// Create a client to talk to our apiserver using the loopback address
	// since we are in the same process as the api server.
	mobileClient, err := clientset.NewForConfig(s.GenericAPIServer.LoopbackClientConfig)

	// install the open service broker spec api routes
	brokerOps := &operations.BrokerOperations{Client: mobileClient}
	broker.Route(s.GenericAPIServer.HandlerContainer.Container, broker.BrokerAPIPrefix, brokerOps)
	glog.Info("Finished installing broker api endpoints")
	//mobileroutes.RouteSDK(s.GenericAPIServer.HandlerContainer.Container, "/sdk", mobileroutes.NewSDKHandler())

	return s, nil
}
