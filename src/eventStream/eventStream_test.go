/*-
 * Copyright (c) 2016,2017, F5 Networks, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package eventStream

import (
	"fmt"
	"github.com/stretchr/testify/require"
	goruntime "runtime"
	"testing"
	"time"

	"k8s.io/client-go/1.4/kubernetes/typed/core/v1/fake"
	"k8s.io/client-go/1.4/pkg/api"
	"k8s.io/client-go/1.4/pkg/api/unversioned"
	"k8s.io/client-go/1.4/pkg/api/v1"
	"k8s.io/client-go/1.4/pkg/runtime"
	"k8s.io/client-go/1.4/pkg/watch"
)

func dumpConfigMaps(items []interface{}) {
	for _, item := range items {
		cm := item.(*v1.ConfigMap)
		fmt.Printf("configMap name=%v namespace=%v version=%v\n",
			cm.ObjectMeta.Name, cm.ObjectMeta.Namespace, cm.ObjectMeta.ResourceVersion)
	}
}

func timedChanWait(ch chan bool, timeoutSecs time.Duration) bool {
	timeout := make(chan bool, 1)
	defer close(timeout)
	go func() {
		time.Sleep(timeoutSecs * time.Second)
		timeout <- true
	}()
	select {
	case <-ch:
		return true
	case <-timeout:
		return false
	}
}

func getConfigMap(t *testing.T, store *EventStore, cm *v1.ConfigMap) *v1.ConfigMap {
	item, exists, err := store.Get(cm)
	require.Nil(t, err, "Unable to find object in store, err=%v", err)
	require.True(t, exists, "Expected object to exist in store")
	return item.(*v1.ConfigMap)
}

func newConfigMap(id, namespace, rv string) *v1.ConfigMap {
	return &v1.ConfigMap{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:            id,
			Namespace:       namespace,
			ResourceVersion: rv,
		},
	}
}

func newService(id, namespace, rv string) *v1.Service {
	return &v1.Service{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:            id,
			Namespace:       namespace,
			ResourceVersion: rv,
		},
	}
}

func TestRunnerStartStop(t *testing.T) {
	// This is the existing data on 'startup'
	existingData := []v1.ConfigMap{
		*newConfigMap("configmap0", "test", "0"),
	}

	// Object that satisfies watch.Interface for test purposes
	fakeWatcher := watch.NewFake()
	// Channel that allows us to block until WatchFunc is called
	inWatchChan := make(chan bool, 1)
	defer close(inWatchChan)
	// Create an event stream for *v1.ConfigMap
	eventStream := NewEventStream(
		&EventListWatch{
			ListFunc: func(options api.ListOptions) (runtime.Object, error) {
				return &v1.ConfigMapList{ListMeta: unversioned.ListMeta{ResourceVersion: "1"}, Items: existingData}, nil
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				inWatchChan <- true
				return fakeWatcher, nil
			},
			OnChangeFunc: func(changeType ChangeType, obj interface{}) {
			},
		},
		&v1.ConfigMap{},
		0)
	// Start the event stream
	eventStream.Run()
	defer eventStream.Stop()

	// If it is running its store will contain existingData
	var timeoutSecs time.Duration = 3
	ok := timedChanWait(inWatchChan, timeoutSecs)
	require.True(t, ok, "Did not enter watch phase after %v seconds", timeoutSecs)

	// If it is running its store will contain existingData
	cm := getConfigMap(t, eventStream.Store(), &existingData[0])
	require.NotNil(t, cm, "Unexpected nil ConfigMap from get of %+v", existingData[0])
}

func TestRunnerEndToEnd(t *testing.T) {
	// This is the existing data on 'startup'
	existingData := []v1.ConfigMap{
		*newConfigMap("configmap0", "test", "0"),
		*newConfigMap("configmap1", "test", "1"),
		*newConfigMap("configmap2", "test", "2"),
		*newConfigMap("configmap3", "test", "3"),
		*newConfigMap("configmap4", "test", "4"),
	}

	// Object that satisfies watch.Interface for test purposes
	fakeWatcher := watch.NewFake()
	// Channel that allows us to block until WatchFunc is called
	inWatchChan := make(chan bool, 1)
	defer close(inWatchChan)
	// Channel that allows us to block until OnChangeFunc is called
	inChangeChan := make(chan bool, 1)
	defer close(inChangeChan)
	// Create an event stream for *v1.ConfigMap
	eventStream := NewEventStream(
		&EventListWatch{
			ListFunc: func(options api.ListOptions) (runtime.Object, error) {
				return &v1.ConfigMapList{ListMeta: unversioned.ListMeta{ResourceVersion: "1"}, Items: existingData}, nil
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				inWatchChan <- true
				return fakeWatcher, nil
			},
			OnChangeFunc: func(changeType ChangeType, obj interface{}) {
				inChangeChan <- true
			},
		},
		&v1.ConfigMap{},
		0)
	goRoutinesBefore := goruntime.NumGoroutine()
	eventStream.Run()
	defer eventStream.Stop()

	// Make sure the goroutine is running
	var timeoutSecs time.Duration = 3
	ok := timedChanWait(inWatchChan, timeoutSecs)
	require.True(t, ok, "Did not enter watch phase after %v seconds", timeoutSecs)
	ok = timedChanWait(inChangeChan, timeoutSecs)
	require.True(t, ok, "Did not enter watch phase after %v seconds", timeoutSecs)

	goRoutinesAfter := goruntime.NumGoroutine()
	require.Condition(t, func() bool { return goRoutinesBefore < goRoutinesAfter },
		"Expected # of goroutines to increase after calling eventStream.Run(), before %v, after %v",
		goRoutinesBefore, goRoutinesAfter)
	items := eventStream.Store().List()
	require.Equal(t, len(existingData), len(items), "Expected %v items in store, but got %v",
		len(existingData), len(items))

	// Make sure we can add through the watcher and the store is updated
	lenBefore := len(eventStream.Store().List())
	fakeWatcher.Add(newConfigMap("added", "test", "24"))
	ok = timedChanWait(inChangeChan, timeoutSecs)
	require.True(t, ok, "Did not enter watch phase after %v seconds", timeoutSecs)

	lenAfter := len(eventStream.Store().List())
	require.Equal(t, lenBefore, lenAfter-1, "Expected %v items in store, but got %v",
		lenBefore+1, lenAfter)

	// Make sure we can modify through the watcher and the store is updated
	cm2 := getConfigMap(t, eventStream.Store(), &existingData[2])
	cm2.ObjectMeta.ResourceVersion = "12"
	lenBefore = lenAfter
	fakeWatcher.Modify(cm2)
	ok = timedChanWait(inChangeChan, timeoutSecs)
	require.True(t, ok, "Did not enter watch phase after %v seconds", timeoutSecs)

	lenAfter = len(eventStream.Store().List())
	require.Equal(t, lenBefore, lenAfter, "Expected %v items in store, but got %v", lenBefore, lenAfter)

	// Make sure we can delete through the watcher and the store is updated
	cm3 := getConfigMap(t, eventStream.Store(), &existingData[3])
	fakeWatcher.Delete(cm3)
	ok = timedChanWait(inChangeChan, timeoutSecs)
	require.True(t, ok, "Did not enter watch phase after %v seconds", timeoutSecs)

	items = eventStream.Store().List()
	lenAfter = len(items)
	require.Equal(t, lenBefore, lenAfter+1, "Expected %v items in store, but got %v",
		lenBefore-1, lenAfter)
}

func TestRunnerResourceVersionHandling(t *testing.T) {
	// This is the existing data on 'startup'
	existingData := []v1.ConfigMap{
		*newConfigMap("configmap0", "test", "3"),
		*newConfigMap("configmap0", "test", "1"),
		*newConfigMap("configmap0", "test", "0"),
		*newConfigMap("configmap0", "test", "2"),
		*newConfigMap("configmap0", "test", "4"),
	}

	// Object that satisfies watch.Interface for test purposes
	fakeWatcher := watch.NewFake()
	// Channel that allows us to block until WatchFunc is called
	inWatchChan := make(chan bool, 1)
	defer close(inWatchChan)
	// Create an event stream for *v1.ConfigMap
	eventStream := NewEventStream(
		&EventListWatch{
			ListFunc: func(options api.ListOptions) (runtime.Object, error) {
				return &v1.ConfigMapList{ListMeta: unversioned.ListMeta{ResourceVersion: "1"}, Items: existingData}, nil
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				inWatchChan <- true
				return fakeWatcher, nil
			},
			OnChangeFunc: nil,
		},
		&v1.ConfigMap{},
		0)
	eventStream.Run()
	defer eventStream.Stop()

	var timeoutSecs time.Duration = 3
	ok := timedChanWait(inWatchChan, timeoutSecs)
	require.True(t, ok, "Did not enter watch phase after %v seconds", timeoutSecs)

	// The keys are all the same in existingData, make sure we have only one
	// item in the eventStore and that it is the latest one (highest version)
	items := eventStream.Store().List()
	require.Equal(t, 1, len(items), "Expected 1 item in store, but got %v", len(items))

	cm := getConfigMap(t, eventStream.Store(), &existingData[4])
	require.NotNil(t, cm, "Unexpected nil ConfigMap from get of %+v", existingData[4])
}

func TestNewConfigMapEventStream(t *testing.T) {
	namespace := "testns"
	eventStream := NewConfigMapEventStream(&fake.FakeCore{}, namespace, 0, nil, nil, nil)
	require.NotNil(t, eventStream, "Unexpected nil eventStream")

	eventStore := eventStream.Store()
	require.NotNil(t, eventStore, "Unexpected nil eventStore")

	existingData := []v1.ConfigMap{
		*newConfigMap("configmap0", namespace, "0"),
		*newConfigMap("configmap1", namespace, "1"),
		*newConfigMap("configmap2", namespace, "2"),
		*newConfigMap("configmap3", namespace, "3"),
		*newConfigMap("configmap4", namespace, "4"),
	}
	for _, item := range existingData {
		err := eventStore.Add(&item)
		if err != nil {
			t.Errorf("eventStore.Add() failed, err=%v", err)
		}
	}
	items := eventStore.List()
	require.Equal(t, len(existingData), len(items), "Expected %v items in store, but got %v",
		len(existingData), len(items))
}

func TestNewServiceEventStream(t *testing.T) {
	namespace := "testns"
	eventStream := NewServiceEventStream(&fake.FakeCore{}, namespace, 0, nil, nil, nil)
	require.NotNil(t, eventStream, "Unexpected nil eventStream")

	eventStore := eventStream.Store()
	require.NotNil(t, eventStore, "Unexpected nil eventStore")

	existingData := []v1.Service{
		*newService("service0", namespace, "0"),
		*newService("service1", namespace, "1"),
		*newService("service2", namespace, "2"),
		*newService("service3", namespace, "3"),
		*newService("service4", namespace, "4"),
	}
	for _, item := range existingData {
		err := eventStore.Add(&item)
		require.Nil(t, err, "eventStore.Add() failed, err=%v", err)
	}
	items := eventStore.List()
	require.Equal(t, len(existingData), len(items), "Expected %v items in store, but got %v",
		len(existingData), len(items))
}
