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

package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	kcpclienthelper "github.com/kcp-dev/apimachinery/pkg/client"
	"github.com/kcp-dev/logicalcluster/v2"
	"github.com/stretchr/testify/require"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimachineryerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	apisv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"
	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	cacheclient "github.com/kcp-dev/kcp/pkg/cache/client"
	"github.com/kcp-dev/kcp/pkg/cache/client/shard"
	cacheserver "github.com/kcp-dev/kcp/pkg/cache/server"
	cacheopitons "github.com/kcp-dev/kcp/pkg/cache/server/options"
	clientset "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"
	"github.com/kcp-dev/kcp/pkg/embeddedetcd"
	"github.com/kcp-dev/kcp/test/e2e/fixtures/apifixtures"
	"github.com/kcp-dev/kcp/test/e2e/framework"
)

type testScenario struct {
	name string
	work func(ctx context.Context, t *testing.T, server framework.RunningServer, kcpShardClusterClient clientset.ClusterInterface, cacheKcpClusterClient clientset.ClusterInterface)
}

// scenarios all test scenarios that will be run against in-process and standalone cache server
var scenarios = []testScenario{
	{"TestReplicateAPIExport", replicateAPIExportScenario},
	{"TestReplicateAPIExportNegative", replicateAPIExportNegativeScenario},
	{"TestReplicateAPIResourceSchema", replicateAPIResourceSchemaScenario},
	{"TestReplicateAPIResourceSchemaNegative", replicateAPIResourceSchemaNegativeScenario},
}

// baseScenario an auxiliary struct that is used by replicateResourceScenario
type baseScenario struct {
	server framework.RunningServer

	resourceName string
	resourceKind string

	createSourceResource        func(logicalcluster.Name) error
	getSourceResource           func(logicalcluster.Name) (interface{}, error)
	updateSourceResource        func(logicalcluster.Name, interface{}) (interface{}, error)
	updateSpecForSourceResource func(interface{}) error
	deleteSourceResource        func(logicalcluster.Name) error

	getCachedResource    func(logicalcluster.Name) (interface{}, error)
	deleteCachedResource func(logicalcluster.Name) error
	updateCachedResource func(logicalcluster.Name) error
}

// replicateAPIResourceSchemaScenario tests if an APIResourceSchema is propagated to the cache server.
// The test exercises creation, modification and removal of the APIExport object.
func replicateAPIResourceSchemaScenario(ctx context.Context, t *testing.T, server framework.RunningServer, kcpShardClusterClient clientset.ClusterInterface, cacheKcpClusterClient clientset.ClusterInterface) {
	replicateResourceScenario(t, baseScenario{
		server:       server,
		resourceName: "today.sheriffs.wild.wild.west",
		resourceKind: "APIResourceSchema",
		createSourceResource: func(cluster logicalcluster.Name) error {
			apifixtures.CreateSheriffsSchemaAndExport(ctx, t, cluster, kcpShardClusterClient.Cluster(cluster), "wild.wild.west", "testing replication to the cache server")
			return nil
		},
		getSourceResource: func(cluster logicalcluster.Name) (interface{}, error) {
			return kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Get(ctx, "today.sheriffs.wild.wild.west", metav1.GetOptions{})
		},
		updateSourceResource: func(cluster logicalcluster.Name, res interface{}) (interface{}, error) {
			apiResSchema, ok := res.(*apisv1alpha1.APIResourceSchema)
			if !ok {
				return nil, fmt.Errorf("%T is not *APIResourceSchema", res)
			}
			return kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Update(ctx, apiResSchema, metav1.UpdateOptions{})
		},
		updateSpecForSourceResource: func(res interface{}) error {
			// no-op, spec of an APIResourceSchema is immutable
			// there is an admission enforcing it
			return nil
		},
		deleteSourceResource: func(cluster logicalcluster.Name) error {
			return kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Delete(ctx, "today.sheriffs.wild.wild.west", metav1.DeleteOptions{})
		},
		getCachedResource: func(cluster logicalcluster.Name) (interface{}, error) {
			return cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Get(cacheclient.WithShardInContext(ctx, shard.New("root")), "today.sheriffs.wild.wild.west", metav1.GetOptions{})
		},
	})
}

// replicateAPIResourceSchemaNegativeScenario checks if modified or even deleted cached APIResourceSchema will be reconciled to match the original object
func replicateAPIResourceSchemaNegativeScenario(ctx context.Context, t *testing.T, server framework.RunningServer, kcpShardClusterClient clientset.ClusterInterface, cacheKcpClusterClient clientset.ClusterInterface) {
	replicateResourceNegativeScenario(t, baseScenario{
		server:       server,
		resourceName: "mangodb",
		resourceKind: "APIResourceSchema",
		createSourceResource: func(cluster logicalcluster.Name) error {
			schema := &apisv1alpha1.APIResourceSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "juicy.mangodbs.db.io",
				},
				Spec: apisv1alpha1.APIResourceSchemaSpec{
					Group: "db.io",
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Plural:   "mangodbs",
						Singular: "mangodb",
						Kind:     "MangoDB",
						ListKind: "MangoDBList",
					},
					Scope: "Namespaced",
					Versions: []apisv1alpha1.APIResourceVersion{
						{
							Name:    "v1",
							Served:  true,
							Storage: true,
							Schema: runtime.RawExtension{
								Raw: func() []byte {
									ret, err := json.Marshal(&apiextensionsv1.JSONSchemaProps{
										Type:        "object",
										Description: "the best db out there",
									})
									if err != nil {
										panic(err)
									}

									return ret
								}(),
							},
						},
					},
				},
			}
			_, err := kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Create(ctx, schema, metav1.CreateOptions{})
			return err
		},
		getSourceResource: func(cluster logicalcluster.Name) (interface{}, error) {
			return kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Get(ctx, "juicy.mangodbs.db.io", metav1.GetOptions{})
		},
		getCachedResource: func(cluster logicalcluster.Name) (interface{}, error) {
			return cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Get(cacheclient.WithShardInContext(ctx, shard.New("root")), "juicy.mangodbs.db.io", metav1.GetOptions{})
		},
		deleteCachedResource: func(cluster logicalcluster.Name) error {
			return cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Delete(cacheclient.WithShardInContext(ctx, shard.New("root")), "juicy.mangodbs.db.io", metav1.DeleteOptions{})
		},
		updateCachedResource: func(cluster logicalcluster.Name) error {
			cachedSchema, err := cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Get(cacheclient.WithShardInContext(ctx, shard.New("root")), "juicy.mangodbs.db.io", metav1.GetOptions{})
			if err != nil {
				return err
			}
			// since the spec of an APIResourceSchema is immutable
			// let's modify some metadata
			if cachedSchema.Labels == nil {
				cachedSchema.Labels = map[string]string{}
			}
			cachedSchema.Labels["foo"] = "bar"
			_, err = cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIResourceSchemas().Update(cacheclient.WithShardInContext(ctx, shard.New("root")), cachedSchema, metav1.UpdateOptions{})
			return err
		},
	})
}

// replicateAPIExportScenario tests if an APIExport is propagated to the cache server.
// The test exercises creation, modification and removal of the APIExport object.
func replicateAPIExportScenario(ctx context.Context, t *testing.T, server framework.RunningServer, kcpShardClusterClient clientset.ClusterInterface, cacheKcpClusterClient clientset.ClusterInterface) {
	replicateResourceScenario(t, baseScenario{
		server:       server,
		resourceName: "wild.wild.west",
		resourceKind: "APIExport",
		createSourceResource: func(cluster logicalcluster.Name) error {
			apifixtures.CreateSheriffsSchemaAndExport(ctx, t, cluster, kcpShardClusterClient.Cluster(cluster), "wild.wild.west", "testing replication to the cache server")
			return nil
		},
		getSourceResource: func(cluster logicalcluster.Name) (interface{}, error) {
			return kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Get(ctx, "wild.wild.west", metav1.GetOptions{})
		},
		updateSourceResource: func(cluster logicalcluster.Name, res interface{}) (interface{}, error) {
			apiExport, ok := res.(*apisv1alpha1.APIExport)
			if !ok {
				return nil, fmt.Errorf("%T is not *APIExport", res)
			}
			return kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Update(ctx, apiExport, metav1.UpdateOptions{})
		},
		updateSpecForSourceResource: func(res interface{}) error {
			apiExport, ok := res.(*apisv1alpha1.APIExport)
			if !ok {
				return fmt.Errorf("%T is not *APIExport", res)
			}
			apiExport.Spec.LatestResourceSchemas = append(apiExport.Spec.LatestResourceSchemas, "foo.bar")
			return nil
		},
		deleteSourceResource: func(cluster logicalcluster.Name) error {
			return kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Delete(ctx, "wild.wild.west", metav1.DeleteOptions{})
		},
		getCachedResource: func(cluster logicalcluster.Name) (interface{}, error) {
			return cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Get(cacheclient.WithShardInContext(ctx, shard.New("root")), "wild.wild.west", metav1.GetOptions{})
		},
	})
}

// replicateAPIExportNegativeScenario checks if modified or even deleted cached APIExport will be reconciled to match the original object
func replicateAPIExportNegativeScenario(ctx context.Context, t *testing.T, server framework.RunningServer, kcpShardClusterClient clientset.ClusterInterface, cacheKcpClusterClient clientset.ClusterInterface) {
	replicateResourceNegativeScenario(t, baseScenario{
		server:       server,
		resourceName: "mangodb",
		resourceKind: "APIExport",
		createSourceResource: func(cluster logicalcluster.Name) error {
			export := &apisv1alpha1.APIExport{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mangodb",
				},
			}
			_, err := kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Create(ctx, export, metav1.CreateOptions{})
			return err
		},
		getSourceResource: func(cluster logicalcluster.Name) (interface{}, error) {
			return kcpShardClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Get(ctx, "mangodb", metav1.GetOptions{})
		},
		getCachedResource: func(cluster logicalcluster.Name) (interface{}, error) {
			return cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Get(cacheclient.WithShardInContext(ctx, shard.New("root")), "mangodb", metav1.GetOptions{})
		},
		deleteCachedResource: func(cluster logicalcluster.Name) error {
			return cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Delete(cacheclient.WithShardInContext(ctx, shard.New("root")), "mangodb", metav1.DeleteOptions{})
		},
		updateCachedResource: func(cluster logicalcluster.Name) error {
			cachedExport, err := cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Get(cacheclient.WithShardInContext(ctx, shard.New("root")), "mangodb", metav1.GetOptions{})
			if err != nil {
				return err
			}
			cachedExport.Spec.LatestResourceSchemas = append(cachedExport.Spec.LatestResourceSchemas, "foo")
			_, err = cacheKcpClusterClient.Cluster(cluster).ApisV1alpha1().APIExports().Update(cacheclient.WithShardInContext(ctx, shard.New("root")), cachedExport, metav1.UpdateOptions{})
			return err
		},
	})
}

// replicateResourceScenario tests if a given resource is propagated to the cache server.
// The test exercises creation, modification and removal of the provided resource.
func replicateResourceScenario(t *testing.T, scenario baseScenario) {
	org := framework.NewOrganizationFixture(t, scenario.server)
	cluster := framework.NewWorkspaceFixture(t, scenario.server, org, framework.WithShardConstraints(tenancyv1alpha1.ShardConstraints{Name: "root"}))

	t.Logf("Create %s %s/%s on the root shard", scenario.resourceKind, cluster, scenario.resourceName)
	require.NoError(t, scenario.createSourceResource(cluster))

	t.Logf("Verify that the resource %s %s/%s is propagated to the cached object", scenario.resourceKind, cluster, scenario.resourceName)
	verifyResourceExistence(t, scenario, cluster)

	t.Logf("Verify that a spec update on %s %s/%s is propagated to the cached object", scenario.resourceKind, cluster, scenario.resourceName)
	verifyResourceUpdate(t, scenario, cluster, scenario.updateSpecForSourceResource)

	t.Logf("Verify that a metadata update on %s %s/%s is propagated ot the cached object", scenario.resourceKind, cluster, scenario.resourceName)
	verifyResourceUpdate(t, scenario, cluster, func(originalResource interface{}) error {
		originalResourceMeta, err := meta.Accessor(originalResource)
		if err != nil {
			return err
		}
		annotations := originalResourceMeta.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["testAnnotation"] = "testAnnotationValue"
		originalResourceMeta.SetAnnotations(annotations)
		return nil
	})

	t.Logf("Verify that deleting %s %s/%s leads to removal of the cached object", scenario.resourceKind, cluster, scenario.resourceName)
	require.NoError(t, scenario.deleteSourceResource(cluster))
	framework.Eventually(t, func() (bool, string) {
		_, err := scenario.getCachedResource(cluster)
		if errors.IsNotFound(err) {
			return true, ""
		}
		if err != nil {
			return false, err.Error()
		}
		return false, fmt.Sprintf("replicated %s %s/%s wasn't removed", scenario.resourceKind, cluster, scenario.resourceName)
	}, wait.ForeverTestTimeout, 400*time.Millisecond)
}

// replicateAPIExportNegativeScenario checks if modified or even deleted cached resource will be reconciled to match the original object
func replicateResourceNegativeScenario(t *testing.T, scenario baseScenario) {
	org := framework.NewOrganizationFixture(t, scenario.server)
	cluster := framework.NewWorkspaceFixture(t, scenario.server, org, framework.WithShardConstraints(tenancyv1alpha1.ShardConstraints{Name: "root"}))

	t.Logf("Creating %s %s/%s", scenario.resourceKind, cluster, scenario.resourceName)
	err := scenario.createSourceResource(cluster)
	require.NoError(t, err, "error creating %s %s|%s", scenario.resourceKind, cluster, scenario.resourceName)

	t.Logf("Verify that %s %s/%s is propagated to the cached object", scenario.resourceKind, cluster, scenario.resourceName)
	verifyResourceExistence(t, scenario, cluster)

	t.Logf("Delete %s %s/%s", scenario.resourceKind, cluster, scenario.resourceName)
	err = scenario.deleteCachedResource(cluster)
	require.NoError(t, err)
	t.Logf("Verify that %s %s/%s is propagated to the cached object after deletion", scenario.resourceKind, cluster, scenario.resourceName)
	verifyResourceExistence(t, scenario, cluster)

	t.Logf("Update %s %s/%s so that it differs from the original object", scenario.resourceKind, cluster, scenario.resourceName)
	err = scenario.updateCachedResource(cluster)
	require.NoError(t, err)
	t.Logf("Verify that %s %s/%s is propaged to the cached object after an update", scenario.resourceKind, cluster, scenario.resourceName)
	verifyResourceExistence(t, scenario, cluster)
}

func verifyResourceExistence(t *testing.T, scenario baseScenario, cluster logicalcluster.Name) {
	t.Logf("Get %s %s/%s from the root shard and the cache server for comparison", scenario.resourceKind, cluster, scenario.resourceName)
	framework.Eventually(t, func() (bool, string) {
		originalResource, err := scenario.getSourceResource(cluster)
		if err != nil {
			return false, err.Error()
		}
		cachedResource, err := scenario.getCachedResource(cluster)
		if err != nil {
			if !errors.IsNotFound(err) {
				return true, err.Error()
			}
			return false, err.Error()
		}
		t.Logf("Verify if both the orignal and replicated resources (%s %s/%s) are the same except %s annotation and ResourceVersion after creation", scenario.resourceKind, cluster, scenario.resourceName, genericapirequest.AnnotationKey)
		cachedResourceMeta, err := meta.Accessor(cachedResource)
		if err != nil {
			return false, err.Error()
		}
		if _, found := cachedResourceMeta.GetAnnotations()[genericapirequest.AnnotationKey]; !found {
			t.Fatalf("replicated %s root|%s/%s, doesn't have %s annotation", scenario.resourceKind, cluster, cachedResourceMeta.GetName(), genericapirequest.AnnotationKey)
		}
		delete(cachedResourceMeta.GetAnnotations(), genericapirequest.AnnotationKey)
		if diff := cmp.Diff(cachedResource, originalResource, cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")); len(diff) > 0 {
			return false, fmt.Sprintf("replicated %s root|%s/%s is different that the original", scenario.resourceKind, cluster, cachedResourceMeta.GetName())
		}
		return true, ""
	}, wait.ForeverTestTimeout, 400*time.Millisecond)
}

func verifyResourceUpdate(t *testing.T, scenario baseScenario, cluster logicalcluster.Name, updateSourceResource func(interface{}) error) {
	t.Logf("Update %s/%s/%s on a shard", scenario.resourceKind, cluster, scenario.resourceName)
	framework.Eventually(t, func() (bool, string) {
		originalResource, err := scenario.getSourceResource(cluster)
		if err != nil {
			return false, err.Error()
		}
		if err = updateSourceResource(originalResource); err != nil {
			return false, err.Error()
		}
		_, err = scenario.updateSourceResource(cluster, originalResource)
		if err != nil {
			if !errors.IsConflict(err) {
				return false, fmt.Sprintf("unknow error while updating the cached %s/%s/%s, err: %s", scenario.resourceKind, cluster, scenario.resourceName, err.Error())
			}
			return false, err.Error() // try again
		}
		return true, ""
	}, wait.ForeverTestTimeout, 400*time.Millisecond)
	t.Logf("Get %s/%s/%s from the cache server", scenario.resourceKind, cluster, scenario.resourceName)
	framework.Eventually(t, func() (bool, string) {
		originalResource, err := scenario.getSourceResource(cluster)
		if err != nil {
			return false, err.Error()
		}
		cachedResource, err := scenario.getCachedResource(cluster)
		if err != nil {
			return false, err.Error()
		}
		t.Logf("Verify if both the orignal and replicated resources (%s/%s/%s) are the same except %s annotation and ResourceVersion after modification", scenario.resourceKind, cluster, scenario.resourceName, genericapirequest.AnnotationKey)
		cachedResourceMeta, err := meta.Accessor(cachedResource)
		if err != nil {
			return false, err.Error()
		}
		if _, found := cachedResourceMeta.GetAnnotations()[genericapirequest.AnnotationKey]; !found {
			t.Fatalf("replicated %s root/%s/%s, doesn't have %s annotation", scenario.resourceKind, cluster, cachedResourceMeta.GetName(), genericapirequest.AnnotationKey)
		}
		delete(cachedResourceMeta.GetAnnotations(), genericapirequest.AnnotationKey)
		if diff := cmp.Diff(cachedResource, originalResource, cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")); len(diff) > 0 {
			return false, fmt.Sprintf("replicated %s root/%s/%s is different that the original", scenario.resourceKind, cluster, cachedResourceMeta.GetName())
		}
		return true, ""
	}, wait.ForeverTestTimeout, 400*time.Millisecond)
}

// TestCacheServerInProcess runs all test scenarios against a cache server that runs with a kcp server
func TestCacheServerInProcess(t *testing.T) {
	t.Parallel()
	framework.Suite(t, "control-plane")

	// TODO(p0lyn0mial): switch to framework.SharedKcpServer when caching is turned on by default
	tokenAuthFile := framework.WriteTokenAuthFile(t)
	server := framework.PrivateKcpServer(t,
		framework.WithCustomArguments(append(framework.TestServerArgsWithTokenAuthFile(tokenAuthFile),
			"--run-cache-server=true",
		)...,
		))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	kcpRootShardConfig := server.RootShardSystemMasterBaseConfig(t)
	kcpRootShardClient, err := clientset.NewClusterForConfig(kcpRootShardConfig)
	require.NoError(t, err)
	cacheClientRT := cacheClientRoundTrippersFor(kcpRootShardConfig)
	cacheKcpClusterClient, err := clientset.NewClusterForConfig(cacheClientRT)
	require.NoError(t, err)

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(tt *testing.T) {
			scenario.work(ctx, tt, server, kcpRootShardClient, cacheKcpClusterClient)
		})
	}
}

// TestCacheServerStandalone runs all test scenarios against a standalone cache server
func TestCacheServerStandalone(t *testing.T) {
	t.Parallel()
	framework.Suite(t, "control-plane")

	artifactDir, dataDir, err := framework.ScratchDirs(t)
	require.NoError(t, err)

	cacheServerPortStr, err := framework.GetFreePort(t)
	require.NoError(t, err)
	cacheServerPort, err := strconv.Atoi(cacheServerPortStr)
	require.NoError(t, err)
	cacheServerOptions := cacheopitons.NewOptions(path.Join(dataDir, "cache"))
	cacheServerOptions.SecureServing.BindPort = cacheServerPort
	cacheServerEmbeddedEtcdClientPort, err := framework.GetFreePort(t)
	require.NoError(t, err)
	cacheServerEmbeddedEtcdPeerPort, err := framework.GetFreePort(t)
	require.NoError(t, err)
	cacheServerOptions.EmbeddedEtcd.ClientPort = cacheServerEmbeddedEtcdClientPort
	cacheServerOptions.EmbeddedEtcd.PeerPort = cacheServerEmbeddedEtcdPeerPort
	cacheServerCompletedOptions, err := cacheServerOptions.Complete()
	require.NoError(t, err)
	if errs := cacheServerCompletedOptions.Validate(); len(errs) > 0 {
		require.NoError(t, apimachineryerrors.NewAggregate(errs))
	}
	cacheServerConfig, err := cacheserver.NewConfig(cacheServerCompletedOptions, nil)
	require.NoError(t, err)
	cacheServerCompletedConfig, err := cacheServerConfig.Complete()
	require.NoError(t, err)
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	if cacheServerCompletedConfig.EmbeddedEtcd.Config != nil {
		t.Logf("Starting embedded etcd for the cache server")
		require.NoError(t, embeddedetcd.NewServer(cacheServerCompletedConfig.EmbeddedEtcd).Run(ctx))
	}
	cacheServer, err := cacheserver.NewServer(cacheServerCompletedConfig)
	require.NoError(t, err)
	preparedCachedServer, err := cacheServer.PrepareRun(ctx)
	require.NoError(t, err)
	t.Logf("Starting the cache server")
	go func() {
		// TODO (p0lyn0mial): check readiness of the cache server
		require.NoError(t, preparedCachedServer.Run(ctx))
	}()
	t.Logf("Creating kubeconfig for the cache server at %s", dataDir)
	cacheServerKubeConfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"cache": {
				Server:               fmt.Sprintf("https://localhost:%s", cacheServerPortStr),
				CertificateAuthority: path.Join(dataDir, "cache", "apiserver.crt"),
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"cache": {
				Cluster: "cache",
			},
		},
		CurrentContext: "cache",
	}
	cacheKubeconfigPath := filepath.Join(dataDir, "cache", "cache.kubeconfig")
	err = clientcmd.WriteToFile(cacheServerKubeConfig, cacheKubeconfigPath)
	require.NoError(t, err)

	// TODO(p0lyn0mial): switch to framework.SharedKcpServer when caching is turned on by default
	tokenAuthFile := framework.WriteTokenAuthFile(t)
	server := framework.PrivateKcpServer(t,
		framework.WithCustomArguments(append(framework.TestServerArgsWithTokenAuthFile(tokenAuthFile), "--run-cache-server=true", fmt.Sprintf("--cache-server-kubeconfig-file=%s", cacheKubeconfigPath))...),
		framework.WithScratchDirectories(artifactDir, dataDir),
	)

	kcpRootShardConfig := server.RootShardSystemMasterBaseConfig(t)
	kcpRootShardClient, err := clientset.NewClusterForConfig(kcpRootShardConfig)
	require.NoError(t, err)
	cacheClientConfig := clientcmd.NewNonInteractiveClientConfig(cacheServerKubeConfig, "cache", nil, nil)
	cacheClientRestConfig, err := cacheClientConfig.ClientConfig()
	require.NoError(t, err)
	cacheClientRT := cacheClientRoundTrippersFor(cacheClientRestConfig)
	cacheKcpClusterClient, err := clientset.NewClusterForConfig(cacheClientRT)
	require.NoError(t, err)

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(tt *testing.T) {
			scenario.work(ctx, tt, server, kcpRootShardClient, cacheKcpClusterClient)
		})
	}
}

func cacheClientRoundTrippersFor(cfg *rest.Config) *rest.Config {
	cacheClientRT := cacheclient.WithCacheServiceRoundTripper(rest.CopyConfig(cfg))
	cacheClientRT = cacheclient.WithShardNameFromContextRoundTripper(cacheClientRT)
	cacheClientRT = cacheclient.WithDefaultShardRoundTripper(cacheClientRT, shard.Wildcard)
	kcpclienthelper.SetMultiClusterRoundTripper(cacheClientRT)
	return cacheClientRT
}
