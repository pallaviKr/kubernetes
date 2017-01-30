/*
Copyright 2017 The Kubernetes Authors.

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

// This file was automatically generated by informer-gen

package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
	api_v1 "k8s.io/kubernetes/pkg/api/v1"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	internalinterfaces "k8s.io/kubernetes/pkg/client/informers/informers_generated/internalinterfaces"
	v1 "k8s.io/kubernetes/pkg/client/listers/core/v1"
	time "time"
)

// PersistentVolumeInformer provides access to a shared informer and lister for
// PersistentVolumes.
type PersistentVolumeInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.PersistentVolumeLister
}

type persistentVolumeInformer struct {
	factory internalinterfaces.SharedInformerFactory
}

func newPersistentVolumeInformer(client clientset.Interface, resyncCheck, resyncPeriod time.Duration) cache.SharedIndexInformer {
	sharedIndexInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return client.CoreV1().PersistentVolumes().List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().PersistentVolumes().Watch(options)
			},
		},
		&api_v1.PersistentVolume{},
		resyncCheck,
		resyncPeriod,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	return sharedIndexInformer
}

func (f *persistentVolumeInformer) Informer() cache.SharedIndexInformer {
	return f.factory.VersionedInformerFor(&api_v1.PersistentVolume{}, newPersistentVolumeInformer)
}

func (f *persistentVolumeInformer) Lister() v1.PersistentVolumeLister {
	return v1.NewPersistentVolumeLister(f.Informer().GetIndexer())
}
