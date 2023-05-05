/*
Copyright 2022 The KCP Authors.

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

package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	kcpdynamic "github.com/kcp-dev/client-go/dynamic"
	kcpkubernetesclientset "github.com/kcp-dev/client-go/kubernetes"
	"github.com/kcp-dev/logicalcluster/v3"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	machjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/transport"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	rootphase0 "github.com/kcp-dev/kcp/config/root-phase0"
	"github.com/kcp-dev/kcp/pkg/virtual/denature"
	"github.com/kcp-dev/kcp/pkg/virtual/framework"
	dynamiccontext "github.com/kcp-dev/kcp/pkg/virtual/framework/dynamic/context"
	"github.com/kcp-dev/kcp/pkg/virtual/framework/handler"
	"github.com/kcp-dev/kcp/pkg/virtual/framework/rootapiserver"
	apisv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/apis/v1alpha1"
	corev1alpha1 "github.com/kcp-dev/kcp/sdk/apis/core/v1alpha1"
	kcpinformers "github.com/kcp-dev/kcp/sdk/client/informers/externalversions"
)

func BuildVirtualWorkspace(
	cfg *rest.Config,
	rootPathPrefix string,
	dynamicClusterClient kcpdynamic.ClusterInterface,
	kubeClusterClient kcpkubernetesclientset.ClusterInterface,
	wildcardKcpInformers kcpinformers.SharedInformerFactory,
) ([]rootapiserver.NamedVirtualWorkspace, error) {
	if !strings.HasSuffix(rootPathPrefix, "/") {
		rootPathPrefix += "/"
	}

	logicalClusterResource := apisv1alpha1.APIResourceSchema{}
	if err := rootphase0.Unmarshal("apiresourceschema-logicalclusters.core.kcp.io.yaml", &logicalClusterResource); err != nil {
		return nil, fmt.Errorf("failed to unmarshal logicalclusters resource: %w", err)
	}
	bs, err := json.Marshal(&apiextensionsv1.JSONSchemaProps{
		Type:                   "object",
		XPreserveUnknownFields: pointer.BoolPtr(true),
	})
	if err != nil {
		return nil, err
	}
	for i := range logicalClusterResource.Spec.Versions {
		v := &logicalClusterResource.Spec.Versions[i]
		v.Schema.Raw = bs // wipe schemas. We don't want validation here.
	}

	myScheme := runtime.NewScheme()
	codeFactory := serializer.NewCodecFactory(myScheme)
	myDecoder := codeFactory.UniversalDeserializer()
	myEncoder := machjson.NewSerializer(machjson.DefaultMetaFactory, nil, nil, false)

	workspaceContentReadyCh := make(chan struct{})
	denaturingWSName := denature.VirtualWorkspaceName
	denaturingWS := &handler.VirtualWorkspace{
		RootPathResolver: framework.RootPathResolverFunc(func(urlPath string, context context.Context) (accepted bool, prefixToStrip string, completedContext context.Context) {
			cluster, apiDomain, prefixToStrip, ok := digestUrl(urlPath, rootPathPrefix)
			if !ok {
				return false, "", context
			}

			if cluster.Wildcard {
				// this virtual workspace requires that a specific cluster be provided
				return false, "", context
			}

			// in this case since we're proxying and not consuming this request we *do not* want to strip
			// the cluster prefix
			prefixToStrip = strings.TrimSuffix(prefixToStrip, cluster.Name.Path().RequestPath())

			completedContext = genericapirequest.WithCluster(context, cluster)
			completedContext = dynamiccontext.WithAPIDomainKey(completedContext, apiDomain)
			return true, prefixToStrip, completedContext
		}),
		Authorizer: authorizer.AuthorizerFunc(func(ctx context.Context, a authorizer.Attributes) (authorizer.Decision, string, error) {
			return authorizer.DecisionAllow, "", nil
		}),
		ReadyChecker: framework.ReadyFunc(func() error {
			select {
			case <-workspaceContentReadyCh:
				return nil
			default:
				return fmt.Errorf("%s virtual workspace controllers are not started", denaturingWSName)
			}
		}),
		HandlerFactory: handler.HandlerFactory(func(rootAPIServerConfig genericapiserver.CompletedConfig) (http.Handler, error) {
			if err := rootAPIServerConfig.AddPostStartHook(denaturingWSName, func(hookContext genericapiserver.PostStartHookContext) error {
				defer close(workspaceContentReadyCh)

				for name, informer := range map[string]cache.SharedIndexInformer{
					"logicalclusters": wildcardKcpInformers.Core().V1alpha1().LogicalClusters().Informer(),
				} {
					if !cache.WaitForNamedCacheSync(name, hookContext.StopCh, informer.HasSynced) {
						klog.Background().Error(nil, "informer not synced")
						return nil
					}
				}

				return nil
			}); err != nil {
				return nil, err
			}

			forwardedHost, err := url.Parse(cfg.Host)
			if err != nil {
				return nil, err
			}

			lister := wildcardKcpInformers.Core().V1alpha1().LogicalClusters().Lister()
			return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				cluster, err := genericapirequest.ClusterNameFrom(request.Context())
				if err != nil {
					http.Error(writer, fmt.Sprintf("could not determine cluster for request: %v", err), http.StatusInternalServerError)
					return
				}
				logicalCluster, err := lister.Cluster(cluster).Get(corev1alpha1.LogicalClusterName)
				if err != nil {
					http.Error(writer, fmt.Sprintf("error getting logicalcluster %s|%s: %v", cluster, corev1alpha1.LogicalClusterName, err), http.StatusInternalServerError)
					return
				}

				if logicalCluster.Status.Phase != corev1alpha1.LogicalClusterPhaseReady {
					http.Error(writer, "the underlying cluster is not ready", http.StatusForbidden)
					return
				}

				ctx := request.Context()
				originUser, ok := genericapirequest.UserFrom(ctx)
				if !ok {
					http.Error(writer, "failed to get origin user", http.StatusInternalServerError)
					return
				}
				extra := map[string][]string{}
				for k, v := range originUser.GetExtra() {
					extra[k] = v
				}

				thisCfg := rest.CopyConfig(cfg)
				thisCfg.Impersonate = rest.ImpersonationConfig{
					UserName: originUser.GetName(),
					UID:      originUser.GetUID(),
					Groups:   originUser.GetGroups(),
					Extra:    extra,
				}
				authenticatingTransport, err := rest.TransportFor(thisCfg)
				if err != nil {
					http.Error(writer, fmt.Sprintf("could create round-tripper: %v", err), http.StatusInternalServerError)
					return
				}
				logger := klog.FromContext(request.Context())
				logger = logger.WithValues("url", request.URL, "userName", originUser.GetName())
				body, err := io.ReadAll(request.Body)
				if err != nil {
					logger.Error(err, "Failed to read body", "request", request)
					http.Error(writer, fmt.Sprintf("failed to read body: %v", err), http.StatusInternalServerError)
				}
				var obj runtime.Object
				var clientGVK *schema.GroupVersionKind
				var newBody string
				if len(body) > 0 {
					obj, clientGVK, err = myDecoder.Decode(body, nil, &unstructured.Unstructured{})
					if err != nil {
						logger.Error(err, "Failed to decode body", "request", request)
						http.Error(writer, fmt.Sprintf("failed to decode request body: %v", err), http.StatusInternalServerError)
					}
					logger.Info("Read request body", "clientGVK", clientGVK, "body", body, "obj", obj)
					var bodyBuilder strings.Builder
					myEncoder.Encode(obj, &bodyBuilder)
					newBody = bodyBuilder.String()
					logger.Info("Recoded request body", "clientGVK", clientGVK, "newBody", newBody)
				} else {
					logger.Info("Request ain't got no body")
				}
				request.Body = io.NopCloser(strings.NewReader(newBody))
				request.ContentLength = int64(len(newBody))
				request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(newBody)), nil }
				proxy := &httputil.ReverseProxy{
					Director: func(request *http.Request) {
						for _, header := range []string{
							"Authorization",
							transport.ImpersonateUserHeader,
							transport.ImpersonateUIDHeader,
							transport.ImpersonateGroupHeader,
						} {
							request.Header.Del(header)
						}
						for key := range request.Header {
							if strings.HasPrefix(key, transport.ImpersonateUserExtraHeaderPrefix) {
								request.Header.Del(key)
							}
						}
						request.URL.Scheme = forwardedHost.Scheme
						request.URL.Host = forwardedHost.Host
					},
					ModifyResponse: func(resp *http.Response) error {
						logger.Info("Starting to process response", "transferEncoding", resp.TransferEncoding, "initialContentLength", resp.ContentLength)
						if sliceHas(resp.TransferEncoding, "chunked") {
							return nil // TODO: implement
						}
						body, err := io.ReadAll(resp.Body)
						if err != nil {
							return fmt.Errorf("failed to read response body: %v", err)
						}
						var newBody string
						if len(body) > 0 {
							obj, clientGVK, err = myDecoder.Decode(body, nil, &unstructured.Unstructured{})
							if err != nil {
								return fmt.Errorf("failed to decode response body: %w", err)
							}
							logger.Info("Read response body", "clientGVK", clientGVK, "body", body, "obj", obj)
							var bodyBuilder strings.Builder
							myEncoder.Encode(obj, &bodyBuilder)
							newBody = bodyBuilder.String()
							logger.Info("Recoded response body", "clientGVK", clientGVK, "newBody", newBody)
						} else {
							logger.Info("Response ain't got no body")
						}
						resp.Body = io.NopCloser(strings.NewReader(newBody))
						resp.ContentLength = int64(len(newBody))
						return nil
					},
					Transport: authenticatingTransport,
				}
				proxy.ServeHTTP(writer, request)
			}), nil
		}),
	}

	return []rootapiserver.NamedVirtualWorkspace{
		{Name: denature.VirtualWorkspaceName, VirtualWorkspace: denaturingWS},
	}, nil
}

func digestUrl(urlPath, rootPathPrefix string) (
	cluster genericapirequest.Cluster,
	key dynamiccontext.APIDomainKey,
	logicalPath string,
	accepted bool,
) {
	if !strings.HasPrefix(urlPath, rootPathPrefix) {
		return genericapirequest.Cluster{}, dynamiccontext.APIDomainKey(""), "", false
	}
	withoutRootPathPrefix := strings.TrimPrefix(urlPath, rootPathPrefix)

	// Incoming requests to this virtual workspace will look like:
	//  /services/denature/clusters/<clustername>/apis/apps.kubernetes.io/v1/deployments/...
	//                     └────────────────────────┐
	// Where the withoutRootPathPrefix starts here: ┘
	parts := strings.SplitN(withoutRootPathPrefix, "/", 3)
	if len(parts) < 3 {
		return genericapirequest.Cluster{}, dynamiccontext.APIDomainKey(""), "", false
	}

	if parts[0] != "clusters" {
		return genericapirequest.Cluster{}, dynamiccontext.APIDomainKey(""), "", false // don't accept
	}

	realPath := "/" + parts[2]

	path := logicalcluster.NewPath(parts[1])
	cluster = genericapirequest.Cluster{}
	if path == logicalcluster.Wildcard {
		cluster.Wildcard = true
	} else {
		var ok bool
		cluster.Name, ok = path.Name()
		if !ok {
			return genericapirequest.Cluster{}, "", "", false
		}
	}

	return cluster, dynamiccontext.APIDomainKey("denatureview"), strings.TrimSuffix(urlPath, realPath), true
}

func sliceHas[Elt comparable](sl []Elt, seek Elt) bool {
	for _, elt := range sl {
		if elt == seek {
			return true
		}
	}
	return false
}
