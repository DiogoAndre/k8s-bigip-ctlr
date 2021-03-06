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

package virtualServer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"eventStream"
	log "f5/vlogger"
	"tools/writer"

	"github.com/xeipuuv/gojsonschema"
	"k8s.io/client-go/1.4/kubernetes"
	"k8s.io/client-go/1.4/pkg/api/v1"
)

// Definition of a Big-IP Virtual Server config
// Most of this comes directly from a ConfigMap, with the exception
// of NodePort and Nodes, which are dynamic
// For more information regarding this structure and data model:
//  f5/schemas/bigip-virtual-server_[version].json
type VirtualServerConfig struct {
	VirtualServer struct {
		Backend struct {
			ServiceName     string   `json:"serviceName"`
			ServicePort     int32    `json:"servicePort"`
			PoolMemberPort  int32    `json:"poolMemberPort"`
			PoolMemberAddrs []string `json:"poolMemberAddrs"`
			HealthMonitors  []struct {
				Interval int    `json:"interval,omitempty"`
				Protocol string `json:"protocol"`
				Send     string `json:"send,omitempty"`
				Timeout  int    `json:"timeout,omitempty"`
			} `json:"healthMonitors,omitempty"`
		} `json:"backend"`
		Frontend struct {
			VirtualServerName string `json:"virtualServerName"`
			// Mutual parameter, partition
			Partition string `json:"partition"`

			// VirtualServer parameters
			Balance        string `json:"balance,omitempty"`
			Mode           string `json:"mode,omitempty"`
			VirtualAddress *struct {
				BindAddr string `json:"bindAddr,omitempty"`
				Port     int32  `json:"port,omitempty"`
			} `json:"virtualAddress,omitempty"`
			SslProfile *struct {
				F5ProfileName string `json:"f5ProfileName,omitempty"`
			} `json:"sslProfile,omitempty"`

			// iApp parameters
			IApp                string `json:"iapp,omitempty"`
			IAppPoolMemberTable struct {
				Name    string `json:"name"`
				Columns []struct {
					Name  string `json:"name"`
					Kind  string `json:"kind,omitempty"`
					Value string `json:"value,omitempty"`
				} `json:"columns"`
			} `json:"iappPoolMemberTable,omitempty"`
			IAppOptions map[string]string `json:"iappOptions,omitempty"`
			IAppTables  map[string]struct {
				Columns []string   `json:"columns,omitempty"`
				Rows    [][]string `json:"rows,omitempty"`
			} `json:"iappTables,omitempty"`
			IAppVariables map[string]string `json:"iappVariables,omitempty"`
		} `json:"frontend"`
	} `json:"virtualServer"`
}

type VirtualServerConfigs []*VirtualServerConfig

func (slice VirtualServerConfigs) Len() int {
	return len(slice)
}

func (slice VirtualServerConfigs) Less(i, j int) bool {
	return slice[i].VirtualServer.Backend.ServiceName <
		slice[j].VirtualServer.Backend.ServiceName ||
		(slice[i].VirtualServer.Backend.ServiceName ==
			slice[j].VirtualServer.Backend.ServiceName &&
			slice[i].VirtualServer.Backend.ServicePort <
				slice[j].VirtualServer.Backend.ServicePort)
}

func (slice VirtualServerConfigs) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

// Indicator to use an F5 schema
var schemaIndicator string = "f5schemadb://"

// Where the schemas reside locally
var schemaLocal string = "file:///app/vendor/src/f5/schemas/"

// Virtual Server Key - unique server is Name + Port
type serviceKey struct {
	ServiceName string
	ServicePort int32
	Namespace   string
}

// Map of Virtual Server configs
var virtualServers struct {
	sync.Mutex
	m map[serviceKey]*VirtualServerConfig
}

// Nodes from previous iteration of node polling
var oldNodes = []string{}

// Mutex to control access to node data
// FIXME: Simple synchronization for now, it remains to be determined if we'll
// need something more complicated (channels, etc?)
var mutex = &sync.Mutex{}

var config writer.Writer
var namespace = ""
var useNodeInternal = false

func SetConfigWriter(cw writer.Writer) {
	config = cw
}

func SetNamespace(ns string) {
	namespace = ns
}

func SetUseNodeInternal(ni bool) {
	useNodeInternal = ni
}

// Package init
func init() {
	virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
}

// Unmarshal an expected VirtualServerConfig object
func parseVirtualServerConfig(cm *v1.ConfigMap) (*VirtualServerConfig, error) {
	var cfg VirtualServerConfig

	if schemaName, ok := cm.Data["schema"]; ok {
		if data, ok := cm.Data["data"]; ok {
			// FIXME For now, "f5schemadb" means the schema is local
			// Trim whitespace and embedded quotes
			schemaName = strings.TrimSpace(schemaName)
			schemaName = strings.Trim(schemaName, "\"")
			if strings.HasPrefix(schemaName, schemaIndicator) {
				schemaName = strings.Replace(schemaName, schemaIndicator, schemaLocal, 1)
			}
			// Load the schema
			schemaLoader := gojsonschema.NewReferenceLoader(schemaName)
			schema, err := gojsonschema.NewSchema(schemaLoader)
			if err != nil {
				return nil, err
			}
			// Load the ConfigMap data and validate
			dataLoader := gojsonschema.NewStringLoader(data)
			result, err := schema.Validate(dataLoader)
			if err != nil {
				return nil, err
			}

			if result.Valid() {
				err := json.Unmarshal([]byte(data), &cfg)
				if nil != err {
					return nil, err
				}
			} else {
				var errors []string
				for _, desc := range result.Errors() {
					errors = append(errors, desc.String())
				}
				return nil, fmt.Errorf("configMap is not valid, errors: %q", errors)
			}
		} else {
			return nil, fmt.Errorf("configmap %s does not contain data key",
				cm.ObjectMeta.Name)
		}
	} else {
		return nil, fmt.Errorf("configmap %s does not contain schema key",
			cm.ObjectMeta.Name)
	}

	return &cfg, nil
}

// Process Service objects from the eventStream
func ProcessServiceUpdate(
	kubeClient kubernetes.Interface,
	changeType eventStream.ChangeType,
	obj interface{},
	isNodePort bool,
	endptStore *eventStream.EventStore) {

	updated := false

	if changeType == eventStream.Replaced {
		v := obj.([]interface{})
		log.Debugf("ProcessServiceUpdate (%v) for %v Services", changeType, len(v))
		for _, item := range v {
			updated = processService(kubeClient, changeType, item, isNodePort, endptStore) || updated
		}
	} else {
		log.Debugf("ProcessServiceUpdate (%v) for 1 Service", changeType)
		updated = processService(kubeClient, changeType, obj, isNodePort, endptStore) || updated
	}

	if updated {
		// Output the Big-IP config
		outputConfig()
	}
}

// Process ConfigMap objects from the eventStream
func ProcessConfigMapUpdate(
	kubeClient kubernetes.Interface,
	changeType eventStream.ChangeType,
	obj interface{},
	isNodePort bool,
	endptStore *eventStream.EventStore) {

	updated := false

	if changeType == eventStream.Replaced {
		v := obj.([]interface{})
		for _, item := range v {
			log.Debugf("ProcessConfigMapUpdate (%v) for %v ConfigMaps", changeType, len(v))
			updated = processConfigMap(kubeClient, changeType, item, isNodePort, endptStore) || updated
		}
	} else {
		log.Debugf("ProcessConfigMapUpdate (%v) for 1 ConfigMap", changeType)
		updated = processConfigMap(kubeClient, changeType, obj, isNodePort, endptStore) || updated
	}

	if updated {
		// Output the Big-IP config
		outputConfig()
	}
}

func ProcessEndpointsUpdate(
	kubeClient kubernetes.Interface,
	changeType eventStream.ChangeType,
	obj interface{},
	serviceStore *eventStream.EventStore) {

	updated := false

	if changeType == eventStream.Replaced {
		v := obj.([]interface{})
		log.Debugf("ProcessEndpointsUpdate (%v) for %v Pod", changeType, len(v))
		for _, item := range v {
			updated = processEndpoints(kubeClient, changeType, item, serviceStore) || updated
		}
	} else {
		log.Debugf("ProcessEndpointsUpdate (%v) for 1 Pod", changeType)
		updated = processEndpoints(kubeClient, changeType, obj, serviceStore) || updated
	}

	if updated {
		// Output the Big-IP config
		outputConfig()
	}
}

func getEndpointsForService(
	portName string,
	eps *v1.Endpoints,
) []string {
	// FIXME(yacobucci) #87
	// we could pass back the nil ips but _f5.py crashes when poolMemberAddrs
	// is json:null. we can protect _f5.py by making this json:[] when empty
	var ipPorts []string = []string{}

	for _, subset := range eps.Subsets {
		for _, p := range subset.Ports {
			if portName == p.Name {
				port := strconv.Itoa(int(p.Port))
				for _, addr := range subset.Addresses {
					var b bytes.Buffer
					b.WriteString(addr.IP)
					b.WriteRune(':')
					b.WriteString(port)
					ipPorts = append(ipPorts, b.String())
				}
			}
		}
	}
	if 0 != len(ipPorts) {
		sort.Strings(ipPorts)
	}
	return ipPorts
}

// Process a change in Service state
func processService(
	kubeClient kubernetes.Interface,
	changeType eventStream.ChangeType,
	obj interface{},
	isNodePort bool,
	endptStore *eventStream.EventStore) bool {

	var svc *v1.Service
	rmvdPortsMap := make(map[int32]*struct{})
	o, ok := obj.(eventStream.ChangedObject)
	if !ok {
		svc = obj.(*v1.Service)
	} else {
		switch changeType {
		case eventStream.Added:
			svc = o.New.(*v1.Service)
		case eventStream.Updated:
			svc = o.New.(*v1.Service)
			oldSvc := o.Old.(*v1.Service)

			for _, o := range oldSvc.Spec.Ports {
				rmvdPortsMap[o.Port] = nil
			}
		case eventStream.Deleted:
			svc = o.Old.(*v1.Service)
		}
	}

	serviceName := svc.ObjectMeta.Name
	updateConfig := false

	if svc.ObjectMeta.Namespace != namespace {
		log.Warningf("Recieving service updates for unwatched namespace %s", svc.ObjectMeta.Namespace)
		return false
	}

	// Check if the service that changed is associated with a ConfigMap
	virtualServers.Lock()
	defer virtualServers.Unlock()
	for _, portSpec := range svc.Spec.Ports {
		if vs, ok := virtualServers.m[serviceKey{serviceName, portSpec.Port, namespace}]; ok {
			delete(rmvdPortsMap, portSpec.Port)
			switch changeType {
			case eventStream.Added, eventStream.Replaced, eventStream.Updated:
				if isNodePort {
					if svc.Spec.Type == v1.ServiceTypeNodePort {
						log.Debugf("Service backend matched %+v: using node port %v",
							serviceKey{serviceName, portSpec.Port, namespace}, portSpec.NodePort)

						vs.VirtualServer.Backend.PoolMemberPort = portSpec.NodePort
						vs.VirtualServer.Backend.PoolMemberAddrs = getNodesFromCache()
						updateConfig = true
					}
				} else {
					item, _, err := endptStore.GetByKey(namespace + "/" + serviceName)
					if nil != item {
						eps := item.(*v1.Endpoints)
						ipPorts := getEndpointsForService(portSpec.Name, eps)

						log.Debugf("Found endpoints for backend %+v: %v",
							serviceKey{serviceName, portSpec.Port, namespace}, ipPorts)

						vs.VirtualServer.Backend.PoolMemberPort,
							vs.VirtualServer.Backend.PoolMemberAddrs = 0, ipPorts
						updateConfig = true
					} else {
						log.Debugf("No endpoints for backend %+v: %v",
							serviceKey{serviceName, portSpec.Port, namespace}, err)
					}
				}
			case eventStream.Deleted:
				vs.VirtualServer.Backend.PoolMemberPort = -1
				vs.VirtualServer.Backend.PoolMemberAddrs = nil
				updateConfig = true
			}
		}
	}
	for p, _ := range rmvdPortsMap {
		if vs, ok := virtualServers.m[serviceKey{serviceName, p, namespace}]; ok {
			vs.VirtualServer.Backend.PoolMemberPort = -1
			vs.VirtualServer.Backend.PoolMemberAddrs = nil
			updateConfig = true
		}
	}

	return updateConfig
}

// Process a change in ConfigMap state
func processConfigMap(
	kubeClient kubernetes.Interface,
	changeType eventStream.ChangeType,
	obj interface{},
	isNodePort bool,
	endptStore *eventStream.EventStore) bool {

	var cfg *VirtualServerConfig

	verified := false

	var cm *v1.ConfigMap
	var oldCm *v1.ConfigMap
	o, ok := obj.(eventStream.ChangedObject)
	if !ok {
		cm = obj.(*v1.ConfigMap)
	} else {
		switch changeType {
		case eventStream.Added:
			cm = o.New.(*v1.ConfigMap)
		case eventStream.Updated:
			cm = o.New.(*v1.ConfigMap)
			oldCm = o.Old.(*v1.ConfigMap)
		case eventStream.Deleted:
			cm = o.Old.(*v1.ConfigMap)
		}
	}

	if cm.ObjectMeta.Namespace != namespace {
		log.Warningf("Recieving config map updates for unwatched namespace %s", cm.ObjectMeta.Namespace)
		return false
	}

	// Decode the JSON data in the ConfigMap
	cfg, err := parseVirtualServerConfig(cm)
	if nil != err {
		log.Warningf("Could not get config for ConfigMap: %v - %v",
			cm.ObjectMeta.Name, err)
		return false
	}

	serviceName := cfg.VirtualServer.Backend.ServiceName
	servicePort := cfg.VirtualServer.Backend.ServicePort

	switch changeType {
	case eventStream.Added, eventStream.Replaced, eventStream.Updated:
		// FIXME(yacobucci) Issue #13 this shouldn't go to the API server but
		// use the eventStream and eventStore functionality
		svc, err := kubeClient.Core().Services(namespace).Get(serviceName)

		if nil == err {
			// Check if service is of type NodePort
			if isNodePort {
				if svc.Spec.Type == v1.ServiceTypeNodePort {
					for _, portSpec := range svc.Spec.Ports {
						if portSpec.Port == servicePort {
							log.Debugf("Service backend matched %+v: using node port %v",
								serviceKey{serviceName, portSpec.Port, namespace}, portSpec.NodePort)

							cfg.VirtualServer.Backend.PoolMemberPort = portSpec.NodePort
							cfg.VirtualServer.Backend.PoolMemberAddrs = getNodesFromCache()
						}
					}
				}
			} else {
				item, _, _ := endptStore.GetByKey(namespace + "/" + serviceName)
				if nil != item {
					eps := item.(*v1.Endpoints)
					for _, portSpec := range svc.Spec.Ports {
						if portSpec.Port == servicePort {
							ipPorts := getEndpointsForService(portSpec.Name, eps)

							log.Debugf("Found endpoints for backend %+v: %v",
								serviceKey{serviceName, portSpec.Port, namespace}, ipPorts)

							cfg.VirtualServer.Backend.PoolMemberPort,
								cfg.VirtualServer.Backend.PoolMemberAddrs = 0, ipPorts
						}
					}
				} else {
					log.Debugf("No endpoints for backend %+v: %v",
						serviceKey{serviceName, servicePort, namespace}, err)
				}
			}
		}

		var oldCfg *VirtualServerConfig
		backendChange := false
		if eventStream.Updated == changeType {
			oldCfg, err = parseVirtualServerConfig(oldCm)
			if nil != err {
				log.Warningf("Cannot parse previous value for ConfigMap %s",
					oldCm.ObjectMeta.Name)
			} else {
				oldName := oldCfg.VirtualServer.Backend.ServiceName
				oldPort := oldCfg.VirtualServer.Backend.ServicePort
				if oldName != cfg.VirtualServer.Backend.ServiceName ||
					oldPort != cfg.VirtualServer.Backend.ServicePort {
					backendChange = true
				}
			}
		}

		virtualServers.Lock()
		defer virtualServers.Unlock()
		if eventStream.Added == changeType {
			if _, ok := virtualServers.m[serviceKey{serviceName, servicePort, namespace}]; ok {
				log.Warningf(
					"Overwriting existing entry for backend %+v - change type: %v",
					serviceKey{serviceName, servicePort, namespace}, changeType)
			}
		} else if eventStream.Updated == changeType && true == backendChange {
			if _, ok := virtualServers.m[serviceKey{serviceName, servicePort, namespace}]; ok {
				log.Warningf(
					"Overwriting existing entry for backend %+v - change type: %v",
					serviceKey{serviceName, servicePort, namespace}, changeType)
			}
			delete(virtualServers.m,
				serviceKey{oldCfg.VirtualServer.Backend.ServiceName,
					oldCfg.VirtualServer.Backend.ServicePort, namespace})
		}
		name := fmt.Sprintf("%v_%v", namespace, cm.ObjectMeta.Name)
		cfg.VirtualServer.Frontend.VirtualServerName = name
		virtualServers.m[serviceKey{serviceName, servicePort, namespace}] = cfg
		verified = true
	case eventStream.Deleted:
		virtualServers.Lock()
		defer virtualServers.Unlock()
		delete(virtualServers.m, serviceKey{serviceName, servicePort, namespace})
		verified = true
	}

	return verified
}

func processEndpoints(
	kubeClient kubernetes.Interface,
	changeType eventStream.ChangeType,
	obj interface{},
	serviceStore *eventStream.EventStore) bool {

	var eps *v1.Endpoints
	o, ok := obj.(eventStream.ChangedObject)
	if !ok {
		eps = obj.(*v1.Endpoints)
	} else {
		switch changeType {
		case eventStream.Added, eventStream.Updated, eventStream.Replaced:
			eps = o.New.(*v1.Endpoints)
		case eventStream.Deleted:
			eps = o.Old.(*v1.Endpoints)
		}
	}

	serviceName := eps.ObjectMeta.Name
	namespace := eps.ObjectMeta.Namespace
	item, _, _ := serviceStore.GetByKey(namespace + "/" + serviceName)
	if nil == item {
		return false
	}
	svc := item.(*v1.Service)

	virtualServers.Lock()
	defer virtualServers.Unlock()

	updateConfig := false
	for _, portSpec := range svc.Spec.Ports {
		if vs, ok := virtualServers.m[serviceKey{serviceName, portSpec.Port, namespace}]; ok {
			switch changeType {
			case eventStream.Added, eventStream.Updated, eventStream.Replaced:
				ipPorts := getEndpointsForService(portSpec.Name, eps)
				if !reflect.DeepEqual(ipPorts, vs.VirtualServer.Backend.PoolMemberAddrs) {

					log.Debugf("Updating endpoints for backend: %+v: from %v to %v",
						serviceKey{serviceName, portSpec.Port, namespace},
						vs.VirtualServer.Backend.PoolMemberAddrs, ipPorts)

					vs.VirtualServer.Backend.PoolMemberPort,
						vs.VirtualServer.Backend.PoolMemberAddrs = 0, ipPorts
					updateConfig = true
				}
			case eventStream.Deleted:
				vs.VirtualServer.Backend.PoolMemberAddrs = nil
				vs.VirtualServer.Backend.PoolMemberPort = -1
				updateConfig = true
			}
		}
	}

	return updateConfig
}

// Check for a change in Node state
func ProcessNodeUpdate(obj interface{}, err error) {
	if nil != err {
		log.Warningf("Unable to get list of nodes, err=%+v", err)
		return
	}

	newNodes, err := getNodeAddresses(obj)
	if nil != err {
		log.Warningf("Unable to get list of nodes, err=%+v", err)
		return
	}
	sort.Strings(newNodes)

	virtualServers.Lock()
	defer virtualServers.Unlock()
	mutex.Lock()
	defer mutex.Unlock()
	// Compare last set of nodes with new one
	if !reflect.DeepEqual(newNodes, oldNodes) {
		log.Infof("ProcessNodeUpdate: Change in Node state detected")
		for _, vs := range virtualServers.m {
			vs.VirtualServer.Backend.PoolMemberAddrs = newNodes
		}
		// Output the Big-IP config
		outputConfigLocked()

		// Update node cache
		oldNodes = newNodes
	}
}

// Dump out the Virtual Server configs to a file
func outputConfig() {
	virtualServers.Lock()
	outputConfigLocked()
	virtualServers.Unlock()
}

// Dump out the Virtual Server configs to a file
// This function MUST be called with the virtualServers
// lock held.
func outputConfigLocked() {

	// Initialize the Services array as empty; json.Marshal() writes
	// an uninitialized array as 'null', but we want an empty array
	// written as '[]' instead
	services := VirtualServerConfigs{}

	// Filter the configs to only those that have active services
	for _, vs := range virtualServers.m {
		if vs.VirtualServer.Backend.PoolMemberPort != -1 {
			services = append(services, vs)
		}
	}

	doneCh, errCh, err := config.SendSection("services", services)
	if nil != err {
		log.Warningf("Failed to write Big-IP config data: %v", err)
	} else {
		select {
		case <-doneCh:
			log.Infof("Wrote %v Virtual Server configs", len(services))
			if log.LL_DEBUG == log.GetLogLevel() {
				output, err := json.Marshal(services)
				if nil != err {
					log.Warningf("Failed creating output debug log: %v", err)
				} else {
					log.Debugf("Services: %s", output)
				}
			}
		case e := <-errCh:
			log.Warningf("Failed to write Big-IP config data: %v", e)
		case <-time.After(time.Second):
			log.Warning("Did not receive config write response in 1s")
		}
	}
}

// Return a copy of the node cache
func getNodesFromCache() []string {
	mutex.Lock()
	defer mutex.Unlock()
	nodes := oldNodes

	return nodes
}

// Get a list of Node addresses
func getNodeAddresses(obj interface{}) ([]string, error) {
	nodes, ok := obj.([]v1.Node)
	if false == ok {
		return nil,
			fmt.Errorf("poll update unexpected type, interface is not []v1.Node")
	}

	addrs := []string{}

	var addrType v1.NodeAddressType
	if useNodeInternal {
		addrType = v1.NodeInternalIP
	} else {
		addrType = v1.NodeExternalIP
	}

	for _, node := range nodes {
		if node.Spec.Unschedulable {
			// Skip master node
			continue
		} else {
			nodeAddrs := node.Status.Addresses
			for _, addr := range nodeAddrs {
				if addr.Type == addrType {
					addrs = append(addrs, addr.Address)
				}
			}
		}
	}

	return addrs, nil
}
