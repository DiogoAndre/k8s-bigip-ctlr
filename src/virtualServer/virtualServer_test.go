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
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"eventStream"
	"test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/1.4/kubernetes/fake"
	"k8s.io/client-go/1.4/pkg/api"
	"k8s.io/client-go/1.4/pkg/api/unversioned"
	"k8s.io/client-go/1.4/pkg/api/v1"
	"k8s.io/client-go/1.4/tools/cache"
)

func init() {
	namespace = "default"

	workingDir, _ := os.Getwd()
	schemaUrl = "file://" + workingDir + "/../../vendor/src/f5/schemas/bigip-virtual-server_v0.1.2.json"
}

var schemaUrl string

var configmapFoo string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "foo",
      "servicePort": 80,
      "healthMonitors": [ {
        "interval": 30,
        "timeout": 20,
        "send": "GET /",
        "protocol": "tcp"
        }
      ]
    },
    "frontend": {
      "balance": "round-robin",
      "mode": "http",
      "partition": "velcro",
      "virtualAddress": {
        "bindAddr": "10.128.10.240",
        "port": 5051
      },
      "sslProfile": {
        "f5ProfileName": "velcro/testcert"
      }
    }
  }
}`)

var configmapFoo8080 string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "foo",
      "servicePort": 8080
    },
    "frontend": {
      "balance": "round-robin",
      "mode": "http",
      "partition": "velcro",
      "virtualAddress": {
        "bindAddr": "10.128.10.240",
        "port": 5051
      }
    }
  }
}`)

var configmapFoo9090 string = string(`{
	"virtualServer": {
		"backend": {
			"serviceName": "foo",
			"servicePort": 9090
		},
		"frontend": {
			"balance": "round-robin",
			"mode": "tcp",
			"partition": "velcro",
			"virtualAddress": {
				"bindAddr": "10.128.10.200",
				"port": 4041
			}
		}
	}
}`)

var configmapFooTcp string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "foo",
      "servicePort": 80
    },
    "frontend": {
      "balance": "round-robin",
      "mode": "tcp",
      "partition": "velcro",
      "virtualAddress": {
        "bindAddr": "10.128.10.240",
        "port": 5051
      }
    }
  }
}`)

var configmapBar string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "bar",
      "servicePort": 80
    },
    "frontend": {
      "balance": "round-robin",
      "mode": "http",
      "partition": "velcro",
      "virtualAddress": {
        "bindAddr": "10.128.10.240",
        "port": 6051
      }
    }
  }
}`)

var configmapIApp1 string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "iapp1",
      "servicePort": 80
    },
    "frontend": {
      "partition": "velcro",
      "iapp": "/Common/f5.http",
      "iappPoolMemberTable": {
        "name": "pool__members",
        "columns": [
          {"name": "IPAddress", "kind": "IPAddress"},
          {"name": "Port", "kind": "Port"},
          {"name": "ConnectionLimit", "value": "0"},
          {"name": "SomeOtherValue", "value": "value-1"}
        ]
      },
      "iappOptions": {
        "description": "iApp 1"
      },
      "iappVariables": {
        "monitor__monitor": "/#create_new#",
        "monitor__resposne": "none",
        "monitor__uri": "/",
        "net__client_mode": "wan",
        "net__server_mode": "lan",
        "pool__addr": "127.0.0.1",
        "pool__pool_to_use": "/#create_new#",
        "pool__port": "8080"
      }
    }
  }
}`)

var configmapIApp2 string = string(`{
  "virtualServer": {
    "backend": {
      "serviceName": "iapp2",
      "servicePort": 80
    },
    "frontend": {
      "partition": "velcro",
      "iapp": "/Common/f5.http",
      "iappOptions": {
        "description": "iApp 2"
      },
      "iappTables": {
        "pool__Pools": {
          "columns": ["Index", "Name", "Description", "LbMethod", "Monitor",
                      "AdvOptions"],
          "rows": [["0", "", "", "round-robin", "0", "none"]]
        },
        "monitor__Monitors": {
          "columns": ["Index", "Name", "Type", "Options"],
          "rows": [["0", "/Common/tcp", "none", "none"]]
        }
      },
      "iappPoolMemberTable": {
        "name": "pool__members",
        "columns": [
          {"name": "IPAddress", "kind": "IPAddress"},
          {"name": "Port", "kind": "Port"},
          {"name": "ConnectionLimit", "value": "0"},
          {"name": "SomeOtherValue", "value": "value-1"}
        ]
      },
      "iappVariables": {
        "monitor__monitor": "/#create_new#",
        "monitor__resposne": "none",
        "monitor__uri": "/",
        "net__client_mode": "wan",
        "net__server_mode": "lan",
        "pool__addr": "127.0.0.2",
        "pool__pool_to_use": "/#create_new#",
        "pool__port": "4430"
      }
    }
  }
}`)

var emptyConfig string = string(`{"services":[]}`)

var twoSvcsFourPortsThreeNodesConfig string = string(`{"services":[{"virtualServer":{"backend":{"serviceName":"bar","servicePort":80,"poolMemberPort":37001,"poolMemberAddrs":["127.0.0.1","127.0.0.2","127.0.0.3"]},"frontend":{"virtualServerName":"default_barmap","partition":"velcro","balance":"round-robin","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}}}},{"virtualServer":{"backend":{"healthMonitors":[{"interval":30,"protocol":"tcp","send":"GET /","timeout":20}],"serviceName":"foo","servicePort":80,"poolMemberPort":30001,"poolMemberAddrs":["127.0.0.1","127.0.0.2","127.0.0.3"]},"frontend":{"virtualServerName":"default_foomap","partition":"velcro","balance":"round-robin","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":5051},"sslProfile":{"f5ProfileName":"velcro/testcert"}}}},{"virtualServer":{"backend":{"serviceName":"foo","servicePort":8080,"poolMemberPort":38001,"poolMemberAddrs":["127.0.0.1","127.0.0.2","127.0.0.3"]},"frontend":{"virtualServerName":"default_foomap8080","partition":"velcro","balance":"round-robin","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":5051}}}},{"virtualServer":{"backend":{"serviceName":"foo","servicePort":9090,"poolMemberPort":39001,"poolMemberAddrs":["127.0.0.1","127.0.0.2","127.0.0.3"]},"frontend":{"virtualServerName":"default_foomap9090","partition":"velcro","balance":"round-robin","mode":"tcp","virtualAddress":{"bindAddr":"10.128.10.200","port":4041}}}}]}`)

var twoSvcsTwoNodesConfig string = string(`{"services":[ {"virtualServer":{"backend":{"serviceName":"bar","servicePort":80,"poolMemberPort":37001,"poolMemberAddrs":["127.0.0.1","127.0.0.2"]},"frontend":{"virtualServerName":"default_barmap","balance":"round-robin","mode":"http","partition":"velcro","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}}}},{"virtualServer":{"backend":{"healthMonitors":[{"interval":30,"protocol":"tcp","send":"GET /","timeout":20}],"serviceName":"foo","servicePort":80,"poolMemberPort":30001,"poolMemberAddrs":["127.0.0.1","127.0.0.2"]},"frontend":{"virtualServerName":"default_foomap","balance":"round-robin","mode":"http","partition":"velcro","virtualAddress":{"bindAddr":"10.128.10.240","port":5051},"sslProfile":{"f5ProfileName":"velcro/testcert"}}}}]}`)

var twoSvcsOneNodeConfig string = string(`{"services":[ {"virtualServer":{"backend":{"serviceName":"bar","servicePort":80,"poolMemberPort":37001,"poolMemberAddrs":["127.0.0.3"]},"frontend":{"virtualServerName":"default_barmap","balance":"round-robin","mode":"http","partition":"velcro","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}}}},{"virtualServer":{"backend":{"healthMonitors":[{"interval":30,"protocol":"tcp","send":"GET /","timeout":20}],"serviceName":"foo","servicePort":80,"poolMemberPort":30001,"poolMemberAddrs":["127.0.0.3"]},"frontend":{"virtualServerName":"default_foomap","balance":"round-robin","mode":"http","partition":"velcro","virtualAddress":{"bindAddr":"10.128.10.240","port":5051},"sslProfile":{"f5ProfileName":"velcro/testcert"}}}}]}`)

var oneSvcTwoNodesConfig string = string(`{"services":[ {"virtualServer":{"backend":{"serviceName":"bar","servicePort":80,"poolMemberPort":37001,"poolMemberAddrs":["127.0.0.3"]},"frontend":{"virtualServerName":"default_barmap","balance":"round-robin","mode":"http","partition":"velcro","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}}}}]}`)

var oneSvcOneNodeConfig string = string(`{"services":[{"virtualServer":{"backend":{"serviceName":"bar","servicePort":80,"poolMemberPort":37001,"poolMemberAddrs":["127.0.0.3"]},"frontend":{"virtualServerName":"default_barmap","balance":"round-robin","mode":"http","partition":"velcro","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}}}}]}`)

var twoIappsThreeNodesConfig string = string(`{"services":[{"virtualServer":{"backend":{"serviceName":"iapp1","servicePort":80,"poolMemberPort":10101,"poolMemberAddrs":["192.168.0.1","192.168.0.2","192.168.0.4"]},"frontend":{"virtualServerName":"default_iapp1map","partition":"velcro","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 1"},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.1","pool__pool_to_use":"/#create_new#","pool__port":"8080"}}}},{"virtualServer":{"backend":{"serviceName":"iapp2","servicePort":80,"poolMemberPort":20202,"poolMemberAddrs":["192.168.0.1","192.168.0.2","192.168.0.4"]},"frontend":{"virtualServerName":"default_iapp2map","partition":"velcro","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 2"},"iappTables":{"pool__Pools":{"columns":["Index","Name","Description","LbMethod","Monitor","AdvOptions"],"rows":[["0","","","round-robin","0","none"]]},"monitor__Monitors":{"columns":["Index","Name","Type","Options"],"rows":[["0","/Common/tcp","none","none"]]}},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.2","pool__pool_to_use":"/#create_new#","pool__port":"4430"}}}}]}`)

var twoIappsOneNodeConfig string = string(`{"services":[{"virtualServer":{"backend":{"serviceName":"iapp1","servicePort":80,"poolMemberPort":10101,"poolMemberAddrs":["192.168.0.4"]},"frontend":{"virtualServerName":"default_iapp1map","partition":"velcro","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 1"},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.1","pool__pool_to_use":"/#create_new#","pool__port":"8080"}}}},{"virtualServer":{"backend":{"serviceName":"iapp2","servicePort":80,"poolMemberPort":20202,"poolMemberAddrs":["192.168.0.4"]},"frontend":{"virtualServerName":"default_iapp2map","partition":"velcro","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 2"},"iappTables":{"pool__Pools":{"columns":["Index","Name","Description","LbMethod","Monitor","AdvOptions"],"rows":[["0","","","round-robin","0","none"]]},"monitor__Monitors":{"columns":["Index","Name","Type","Options"],"rows":[["0","/Common/tcp","none","none"]]}},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.2","pool__pool_to_use":"/#create_new#","pool__port":"4430"}}}}]}`)

var oneIappOneNodeConfig string = string(`{"services":[{"virtualServer":{"backend":{"serviceName":"iapp2","servicePort":80,"poolMemberPort":20202,"poolMemberAddrs":["192.168.0.4"]},"frontend":{"virtualServerName":"default_iapp2map","partition":"velcro","iapp":"/Common/f5.http","iappOptions":{"description":"iApp 2"},"iappTables":{"pool__Pools":{"columns":["Index","Name","Description","LbMethod","Monitor","AdvOptions"],"rows":[["0","","","round-robin","0","none"]]},"monitor__Monitors":{"columns":["Index","Name","Type","Options"],"rows":[["0","/Common/tcp","none","none"]]}},"iappPoolMemberTable":{"name":"pool__members","columns":[{"name":"IPAddress","kind":"IPAddress"},{"name":"Port","kind":"Port"},{"name":"ConnectionLimit","value":"0"},{"name":"SomeOtherValue","value":"value-1"}]},"iappVariables":{"monitor__monitor":"/#create_new#","monitor__resposne":"none","monitor__uri":"/","net__client_mode":"wan","net__server_mode":"lan","pool__addr":"127.0.0.2","pool__pool_to_use":"/#create_new#","pool__port":"4430"}}}}]}`)

var twoSvcTwoPodsConfig string = string(`{"services":[{"virtualServer":{"backend":{"serviceName":"bar","servicePort":80,"poolMemberPort":0,"poolMemberAddrs":["10.2.96.0:80","10.2.96.3:80"]},"frontend":{"virtualServerName":"default_barmap","partition":"velcro","balance":"round-robin","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}}}},{"virtualServer":{"backend":{"serviceName":"foo","servicePort":8080,"poolMemberPort":0,"poolMemberAddrs":["10.2.96.1:8080","10.2.96.2:8080"]},"frontend":{"virtualServerName":"default_foomap","partition":"velcro","balance":"round-robin","mode":"http","virtualAddress":{"bindAddr":"10.128.10.240","port":5051}}}}]}`)

var oneSvcTwoPodsConfig string = string(`{"services":[ {"virtualServer":{"backend":{"serviceName":"bar","servicePort":80,"poolMemberPort":0,"poolMemberAddrs":["10.2.96.0:80","10.2.96.3:80"]},"frontend":{"virtualServerName":"default_barmap","balance":"round-robin","mode":"http","partition":"velcro","virtualAddress":{"bindAddr":"10.128.10.240","port":6051}}}}]}`)

func newConfigMap(id, rv, namespace string,
	keys map[string]string) *v1.ConfigMap {
	return &v1.ConfigMap{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:            id,
			ResourceVersion: rv,
			Namespace:       namespace,
		},
		Data: keys,
	}
}

func newService(id, rv, namespace string, serviceType v1.ServiceType,
	portSpecList []v1.ServicePort) *v1.Service {
	return &v1.Service{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:            id,
			ResourceVersion: rv,
			Namespace:       namespace,
		},
		Spec: v1.ServiceSpec{
			Type:  serviceType,
			Ports: portSpecList,
		},
	}
}

func newNode(id, rv string, unsched bool,
	addresses []v1.NodeAddress) *v1.Node {
	return &v1.Node{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:            id,
			ResourceVersion: rv,
		},
		Spec: v1.NodeSpec{
			Unschedulable: unsched,
		},
		Status: v1.NodeStatus{
			Addresses: addresses,
		},
	}
}

func convertSvcPortsToEndpointPorts(svcPorts []v1.ServicePort) []v1.EndpointPort {
	eps := make([]v1.EndpointPort, len(svcPorts))
	for i, v := range svcPorts {
		eps[i].Name = v.Name
		eps[i].Port = v.Port
	}
	return eps
}

func newEndpointAddress(ips []string) []v1.EndpointAddress {
	eps := make([]v1.EndpointAddress, len(ips))
	for i, v := range ips {
		eps[i].IP = v
	}
	return eps
}

func newEndpointPort(portName string, ports []int32) []v1.EndpointPort {
	epp := make([]v1.EndpointPort, len(ports))
	for i, v := range ports {
		epp[i].Name = portName
		epp[i].Port = v
	}
	return epp
}

func newEndpoints(svcName, rv, namespace string,
	readyIps, notReadyIps []string, ports []v1.EndpointPort) *v1.Endpoints {
	ep := &v1.Endpoints{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Endpoints",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:            svcName,
			Namespace:       namespace,
			ResourceVersion: rv,
		},
		Subsets: []v1.EndpointSubset{},
	}

	if 0 < len(readyIps) {
		ep.Subsets = append(
			ep.Subsets,
			v1.EndpointSubset{
				Addresses:         newEndpointAddress(readyIps),
				NotReadyAddresses: newEndpointAddress(notReadyIps),
				Ports:             ports,
			},
		)
	}

	return ep
}

func newServicePort(name string, svcPort int32) v1.ServicePort {
	return v1.ServicePort{
		Port: svcPort,
		Name: name,
	}
}

func newStore(onChange eventStream.OnChangeFunc) *eventStream.EventStore {
	store := eventStream.NewEventStore(cache.MetaNamespaceKeyFunc, onChange)
	return store
}

func TestVirtualServerSendFail(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.ImmediateFail,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	require.NotNil(t, mw)
	assert.True(t, ok)

	require.NotPanics(t, func() {
		outputConfig()
	})
	assert.Equal(t, 1, mw.WrittenTimes)
}

func TestVirtualServerSendFailAsync(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.AsyncFail,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	require.NotNil(t, mw)
	assert.True(t, ok)

	require.NotPanics(t, func() {
		outputConfig()
	})
	assert.Equal(t, 1, mw.WrittenTimes)
}

func TestVirtualServerSendFailTimeout(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Timeout,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	require.NotNil(t, mw)
	assert.True(t, ok)

	require.NotPanics(t, func() {
		outputConfig()
	})
	assert.Equal(t, 1, mw.WrittenTimes)
}

func TestVirtualServerSort(t *testing.T) {
	virtualServers := VirtualServerConfigs{}

	expectedList := make(VirtualServerConfigs, 10)

	vs := VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "bar"
	vs.VirtualServer.Backend.ServicePort = 80
	virtualServers = append(virtualServers, &vs)
	expectedList[1] = &vs

	vs = VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "foo"
	vs.VirtualServer.Backend.ServicePort = 2
	virtualServers = append(virtualServers, &vs)
	expectedList[5] = &vs

	vs = VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "foo"
	vs.VirtualServer.Backend.ServicePort = 8080
	virtualServers = append(virtualServers, &vs)
	expectedList[7] = &vs

	vs = VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "baz"
	vs.VirtualServer.Backend.ServicePort = 1
	virtualServers = append(virtualServers, &vs)
	expectedList[2] = &vs

	vs = VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "foo"
	vs.VirtualServer.Backend.ServicePort = 80
	virtualServers = append(virtualServers, &vs)
	expectedList[6] = &vs

	vs = VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "foo"
	vs.VirtualServer.Backend.ServicePort = 9090
	virtualServers = append(virtualServers, &vs)
	expectedList[9] = &vs

	vs = VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "baz"
	vs.VirtualServer.Backend.ServicePort = 1000
	virtualServers = append(virtualServers, &vs)
	expectedList[3] = &vs

	vs = VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "foo"
	vs.VirtualServer.Backend.ServicePort = 8080
	virtualServers = append(virtualServers, &vs)
	expectedList[8] = &vs

	vs = VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "foo"
	vs.VirtualServer.Backend.ServicePort = 1
	virtualServers = append(virtualServers, &vs)
	expectedList[4] = &vs

	vs = VirtualServerConfig{}
	vs.VirtualServer.Backend.ServiceName = "bar"
	vs.VirtualServer.Backend.ServicePort = 1
	virtualServers = append(virtualServers, &vs)
	expectedList[0] = &vs

	sort.Sort(virtualServers)

	for i, _ := range expectedList {
		require.EqualValues(t, expectedList[i], virtualServers[i],
			"Sorted list elements should be equal")
	}
}

func TestGetAddresses(t *testing.T) {
	// Existing Node data
	expectedNodes := []*v1.Node{
		newNode("node0", "0", true, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.0"}}),
		newNode("node1", "1", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.1"}}),
		newNode("node2", "2", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.2"}}),
		newNode("node3", "3", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.3"}}),
		newNode("node4", "4", false, []v1.NodeAddress{
			{"InternalIP", "127.0.0.4"}}),
		newNode("node5", "5", false, []v1.NodeAddress{
			{"Hostname", "127.0.0.5"}}),
	}

	expectedReturn := []string{
		"127.0.0.1",
		"127.0.0.2",
		"127.0.0.3",
	}

	fake := fake.NewSimpleClientset()
	assert.NotNil(t, fake, "Mock client cannot be nil")

	for _, expectedNode := range expectedNodes {
		node, err := fake.Core().Nodes().Create(expectedNode)
		require.Nil(t, err, "Should not fail creating node")
		require.EqualValues(t, expectedNode, node, "Nodes should be equal")
	}

	useNodeInternal = false
	nodes, err := fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	addresses, err := getNodeAddresses(nodes.Items)
	require.Nil(t, err, "Should not fail getting addresses")
	assert.EqualValues(t, expectedReturn, addresses,
		"Should receive the correct addresses")

	// test filtering
	expectedInternal := []string{
		"127.0.0.4",
	}

	useNodeInternal = true
	addresses, err = getNodeAddresses(nodes.Items)
	require.Nil(t, err, "Should not fail getting internal addresses")
	assert.EqualValues(t, expectedInternal, addresses,
		"Should receive the correct addresses")

	for _, node := range expectedNodes {
		err := fake.Core().Nodes().Delete(node.ObjectMeta.Name,
			&api.DeleteOptions{})
		require.Nil(t, err, "Should not fail deleting node")
	}

	expectedReturn = []string{}
	useNodeInternal = false
	nodes, err = fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	addresses, err = getNodeAddresses(nodes.Items)
	require.Nil(t, err, "Should not fail getting empty addresses")
	assert.EqualValues(t, expectedReturn, addresses, "Should get no addresses")
}

func validateConfig(t *testing.T, mw *test.MockWriter, expected string) {
	mw.Lock()
	_, ok := mw.Sections["services"].(VirtualServerConfigs)
	mw.Unlock()
	assert.True(t, ok)

	services := struct {
		Services VirtualServerConfigs `json:"services"`
	}{
		Services: mw.Sections["services"].(VirtualServerConfigs),
	}

	// Sort virtual-servers configs for comparison
	sort.Sort(services.Services)

	// Read JSON from exepectedOutput into array of structs
	expectedOutput := struct {
		Services VirtualServerConfigs `json:"services"`
	}{
		Services: VirtualServerConfigs{},
	}

	err := json.Unmarshal([]byte(expected), &expectedOutput)
	if nil != err {
		assert.Nil(t, err)
		return
	}

	require.True(t, assert.ObjectsAreEqualValues(expectedOutput, services))
}

func TestProcessNodeUpdate(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	originalSet := []v1.Node{
		*newNode("node0", "0", true, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.0"}}),
		*newNode("node1", "1", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.1"}}),
		*newNode("node2", "2", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.2"}}),
		*newNode("node3", "3", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.3"}}),
		*newNode("node4", "4", false, []v1.NodeAddress{
			{"InternalIP", "127.0.0.4"}}),
		*newNode("node5", "5", false, []v1.NodeAddress{
			{"Hostname", "127.0.0.5"}}),
	}

	expectedOgSet := []string{
		"127.0.0.1",
		"127.0.0.2",
		"127.0.0.3",
	}

	fake := fake.NewSimpleClientset(&v1.NodeList{Items: originalSet})
	assert.NotNil(t, fake, "Mock client should not be nil")

	useNodeInternal = false
	nodes, err := fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)
	require.EqualValues(t, expectedOgSet, oldNodes,
		"Should have cached correct node set")

	cachedNodes := getNodesFromCache()
	require.EqualValues(t, oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedOgSet, cachedNodes,
		"Cached nodes should be expected set")

	// test filtering
	expectedInternal := []string{
		"127.0.0.4",
	}

	useNodeInternal = true
	nodes, err = fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)
	require.EqualValues(t, expectedInternal, oldNodes,
		"Should have cached correct node set")

	cachedNodes = getNodesFromCache()
	require.EqualValues(t, oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedInternal, cachedNodes,
		"Cached nodes should be expected set")

	// add some nodes
	_, err = fake.Core().Nodes().Create(newNode("nodeAdd", "nodeAdd", false,
		[]v1.NodeAddress{{"ExternalIP", "127.0.0.6"}}))
	require.Nil(t, err, "Create should not return err")

	_, err = fake.Core().Nodes().Create(newNode("nodeExclude", "nodeExclude",
		true, []v1.NodeAddress{{"InternalIP", "127.0.0.7"}}))

	useNodeInternal = false
	nodes, err = fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)
	expectedAddSet := append(expectedOgSet, "127.0.0.6")

	require.EqualValues(t, expectedAddSet, oldNodes)

	cachedNodes = getNodesFromCache()
	require.EqualValues(t, oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedAddSet, cachedNodes,
		"Cached nodes should be expected set")

	// make no changes and re-run process
	useNodeInternal = false
	nodes, err = fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)
	expectedAddSet = append(expectedOgSet, "127.0.0.6")

	require.EqualValues(t, expectedAddSet, oldNodes)

	cachedNodes = getNodesFromCache()
	require.EqualValues(t, oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedAddSet, cachedNodes,
		"Cached nodes should be expected set")

	// remove nodes
	err = fake.Core().Nodes().Delete("node1", &api.DeleteOptions{})
	require.Nil(t, err)
	fake.Core().Nodes().Delete("node2", &api.DeleteOptions{})
	require.Nil(t, err)
	fake.Core().Nodes().Delete("node3", &api.DeleteOptions{})
	require.Nil(t, err)

	expectedDelSet := []string{"127.0.0.6"}

	useNodeInternal = false
	nodes, err = fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(t, err, "Should not fail listing nodes")
	ProcessNodeUpdate(nodes.Items, err)
	validateConfig(t, mw, emptyConfig)

	require.EqualValues(t, expectedDelSet, oldNodes)

	cachedNodes = getNodesFromCache()
	require.EqualValues(t, oldNodes, cachedNodes,
		"Cached nodes should be oldNodes")
	require.EqualValues(t, expectedDelSet, cachedNodes,
		"Cached nodes should be expected set")
}

func testOverwriteAddImpl(t *testing.T, isNodePort bool) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)

	cfgFoo := newConfigMap("foomap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})

	fake := fake.NewSimpleClientset()
	require.NotNil(fake, "Mock client cannot be nil")

	endptStore := newStore(nil)
	r := processConfigMap(fake, eventStream.Added,
		eventStream.ChangedObject{nil, cfgFoo}, isNodePort, endptStore)
	require.True(r, "Config map should be processed")

	require.Equal(1, len(virtualServers.m))
	require.Contains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Virtual servers should have entry")
	require.Equal("http",
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Frontend.Mode,
		"Mode should be http")

	cfgFoo = newConfigMap("foomap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFooTcp})

	r = processConfigMap(fake, eventStream.Added,
		eventStream.ChangedObject{nil, cfgFoo}, isNodePort, endptStore)
	require.True(r, "Config map should be processed")

	require.Equal(1, len(virtualServers.m))
	require.Contains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Virtual servers should have new entry")
	require.Equal("tcp",
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Frontend.Mode,
		"Mode should be tcp after overwrite")
}

func TestOverwriteAddNodePort(t *testing.T) {
	testOverwriteAddImpl(t, true)
}

func TestOverwriteAddCluster(t *testing.T) {
	testOverwriteAddImpl(t, false)
}

func testServiceChangeUpdateImpl(t *testing.T, isNodePort bool) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)

	cfgFoo := newConfigMap("foomap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})

	fake := fake.NewSimpleClientset()
	require.NotNil(fake, "Mock client cannot be nil")

	endptStore := newStore(nil)
	r := processConfigMap(fake, eventStream.Added,
		eventStream.ChangedObject{nil, cfgFoo}, true, endptStore)
	require.True(r, "Config map should be processed")

	require.Equal(1, len(virtualServers.m))
	require.Contains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Virtual servers should have an entry")

	cfgFoo8080 := newConfigMap("foomap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})

	r = processConfigMap(fake, eventStream.Updated,
		eventStream.ChangedObject{cfgFoo, cfgFoo8080}, true, endptStore)
	require.True(r, "Config map should be processed")
	require.Contains(virtualServers.m, serviceKey{"foo", 8080, "default"},
		"Virtual servers should have new entry")
	require.NotContains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Virtual servers should have old config removed")
}

func TestServiceChangeUpdateNodePort(t *testing.T) {
	testServiceChangeUpdateImpl(t, true)
}

func TestServiceChangeUpdateCluster(t *testing.T) {
	testServiceChangeUpdateImpl(t, false)
}

func TestServicePortsRemovedNodePort(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)

	cfgFoo := newConfigMap("foomap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgFoo8080 := newConfigMap("foomap8080", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgFoo9090 := newConfigMap("foomap9090", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo9090})

	foo := newService("foo", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001},
			{Port: 8080, NodePort: 38001},
			{Port: 9090, NodePort: 39001}})

	fake := fake.NewSimpleClientset(&v1.ServiceList{Items: []v1.Service{*foo}})

	endptStore := newStore(nil)
	r := processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfgFoo}, true, endptStore)
	require.True(r, "Config map should be processed")

	r = processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfgFoo8080}, true, endptStore)
	require.True(r, "Config map should be processed")

	r = processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfgFoo9090}, true, endptStore)
	require.True(r, "Config map should be processed")

	require.Equal(3, len(virtualServers.m))
	require.Contains(virtualServers.m, serviceKey{"foo", 80, "default"})
	require.Contains(virtualServers.m, serviceKey{"foo", 8080, "default"})
	require.Contains(virtualServers.m, serviceKey{"foo", 9090, "default"})

	// Create a new service with less ports and update
	newFoo := newService("foo", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001}})

	r = processService(fake, eventStream.Updated, eventStream.ChangedObject{
		foo,
		newFoo}, true, endptStore)
	require.True(r, "Service should be processed")

	require.Equal(3, len(virtualServers.m))
	require.Contains(virtualServers.m, serviceKey{"foo", 80, "default"})
	require.Contains(virtualServers.m, serviceKey{"foo", 8080, "default"})
	require.Contains(virtualServers.m, serviceKey{"foo", 9090, "default"})

	require.Equal(int32(30001),
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Backend.PoolMemberPort,
		"Existing NodePort should be set")
	require.Equal(int32(-1),
		virtualServers.m[serviceKey{"foo", 8080, "default"}].VirtualServer.Backend.PoolMemberPort,
		"Removed NodePort should be unset")
	require.Equal(int32(-1),
		virtualServers.m[serviceKey{"foo", 9090, "default"}].VirtualServer.Backend.PoolMemberPort,
		"Removed NodePort should be unset")

	// Re-add port in new service
	newFoo2 := newService("foo", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 20001},
			{Port: 8080, NodePort: 45454}})

	r = processService(fake, eventStream.Updated, eventStream.ChangedObject{
		newFoo,
		newFoo2}, true, endptStore)
	require.True(r, "Service should be processed")

	require.Equal(3, len(virtualServers.m))
	require.Contains(virtualServers.m, serviceKey{"foo", 80, "default"})
	require.Contains(virtualServers.m, serviceKey{"foo", 8080, "default"})
	require.Contains(virtualServers.m, serviceKey{"foo", 9090, "default"})

	require.Equal(int32(20001),
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Backend.PoolMemberPort,
		"Existing NodePort should be set")
	require.Equal(int32(45454),
		virtualServers.m[serviceKey{"foo", 8080, "default"}].VirtualServer.Backend.PoolMemberPort,
		"Removed NodePort should be unset")
	require.Equal(int32(-1),
		virtualServers.m[serviceKey{"foo", 9090, "default"}].VirtualServer.Backend.PoolMemberPort,
		"Removed NodePort should be unset")
}

func TestUpdatesConcurrentNodePort(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	assert := assert.New(t)
	require := require.New(t)

	cfgFoo := newConfigMap("foomap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgBar := newConfigMap("barmap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapBar})
	foo := newService("foo", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001}})
	bar := newService("bar", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	nodes := []*v1.Node{
		newNode("node0", "0", true, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.0"}}),
		newNode("node1", "1", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.1"}}),
		newNode("node2", "2", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.2"}}),
	}
	extraNode := newNode("node3", "3", false,
		[]v1.NodeAddress{{"ExternalIP", "127.0.0.3"}})

	fake := fake.NewSimpleClientset()
	require.NotNil(fake, "Mock client cannot be nil")

	nodeCh := make(chan struct{})
	mapCh := make(chan struct{})
	serviceCh := make(chan struct{})

	go func() {
		for _, node := range nodes {
			n, err := fake.Core().Nodes().Create(node)
			require.Nil(err, "Should not fail creating node")
			require.EqualValues(node, n, "Nodes should be equal")

			useNodeInternal = false
			nodes, err := fake.Core().Nodes().List(api.ListOptions{})
			assert.Nil(err, "Should not fail listing nodes")
			ProcessNodeUpdate(nodes.Items, err)
		}

		nodeCh <- struct{}{}
	}()

	go func() {
		f, err := fake.Core().ConfigMaps("default").Create(cfgFoo)
		require.Nil(err, "Should not fail creating configmap")
		require.EqualValues(f, cfgFoo, "Maps should be equal")

		endptStore := newStore(nil)
		ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
			nil,
			cfgFoo,
		}, true, endptStore)

		b, err := fake.Core().ConfigMaps("default").Create(cfgBar)
		require.Nil(err, "Should not fail creating configmap")
		require.EqualValues(b, cfgBar, "Maps should be equal")

		ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
			nil,
			cfgBar,
		}, true, endptStore)

		mapCh <- struct{}{}
	}()

	go func() {
		fSvc, err := fake.Core().Services("default").Create(foo)
		require.Nil(err, "Should not fail creating service")
		require.EqualValues(fSvc, foo, "Service should be equal")

		endptStore := newStore(nil)
		ProcessServiceUpdate(fake, eventStream.Added, eventStream.ChangedObject{
			nil,
			foo}, true, endptStore)

		bSvc, err := fake.Core().Services("default").Create(bar)
		require.Nil(err, "Should not fail creating service")
		require.EqualValues(bSvc, bar, "Maps should be equal")

		ProcessServiceUpdate(fake, eventStream.Added, eventStream.ChangedObject{
			nil,
			bar}, true, endptStore)

		serviceCh <- struct{}{}
	}()

	select {
	case <-nodeCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting node channel notification")
	}
	select {
	case <-mapCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting configmap channel notification")
	}
	select {
	case <-serviceCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out excpecting service channel notification")
	}

	validateConfig(t, mw, twoSvcsTwoNodesConfig)

	go func() {
		err := fake.Core().Nodes().Delete("node1", &api.DeleteOptions{})
		require.Nil(err)
		err = fake.Core().Nodes().Delete("node2", &api.DeleteOptions{})
		require.Nil(err)
		_, err = fake.Core().Nodes().Create(extraNode)
		require.Nil(err)
		useNodeInternal = false
		nodes, err := fake.Core().Nodes().List(api.ListOptions{})
		assert.Nil(err, "Should not fail listing nodes")
		ProcessNodeUpdate(nodes.Items, err)

		nodeCh <- struct{}{}
	}()

	go func() {
		err := fake.Core().ConfigMaps("default").Delete("foomap",
			&api.DeleteOptions{})
		require.Nil(err, "Should not error deleting map")
		m, _ := fake.Core().ConfigMaps("").List(api.ListOptions{})
		assert.Equal(1, len(m.Items))
		endptStore := newStore(nil)
		ProcessConfigMapUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
			cfgFoo,
			nil,
		}, true, endptStore)
		assert.Equal(1, len(virtualServers.m))

		mapCh <- struct{}{}
	}()

	go func() {
		err := fake.Core().Services("default").Delete("foo",
			&api.DeleteOptions{})
		require.Nil(err, "Should not error deleting service")
		s, _ := fake.Core().Services("").List(api.ListOptions{})
		assert.Equal(1, len(s.Items))
		endptStore := newStore(nil)
		ProcessServiceUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
			foo,
			nil}, true, endptStore)

		serviceCh <- struct{}{}
	}()

	select {
	case <-nodeCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting node channel notification")
	}
	select {
	case <-mapCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting configmap channel notification")
	}
	select {
	case <-serviceCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out excpecting service channel notification")
	}

	validateConfig(t, mw, oneSvcTwoNodesConfig)
}

func TestProcessUpdatesNodePort(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	assert := assert.New(t)
	require := require.New(t)

	// Create a test env with two ConfigMaps, two Services, and three Nodes
	cfgFoo := newConfigMap("foomap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgFoo8080 := newConfigMap("foomap8080", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgFoo9090 := newConfigMap("foomap9090", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo9090})
	cfgBar := newConfigMap("barmap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapBar})
	foo := newService("foo", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001},
			{Port: 8080, NodePort: 38001},
			{Port: 9090, NodePort: 39001}})
	bar := newService("bar", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	nodes := []v1.Node{
		*newNode("node0", "0", true, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.0"}}),
		*newNode("node1", "1", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.1"}}),
		*newNode("node2", "2", false, []v1.NodeAddress{
			{"ExternalIP", "127.0.0.2"}}),
	}
	extraNode := newNode("node3", "3", false,
		[]v1.NodeAddress{{"ExternalIP", "127.0.0.3"}})

	addrs := []string{"127.0.0.1", "127.0.0.2"}

	fake := fake.NewSimpleClientset(
		&v1.ConfigMapList{Items: []v1.ConfigMap{*cfgFoo, *cfgBar}},
		&v1.ServiceList{Items: []v1.Service{*foo, *bar}},
		&v1.NodeList{Items: nodes})
	require.NotNil(fake, "Mock client cannot be nil")

	m, err := fake.Core().ConfigMaps("").List(api.ListOptions{})
	require.Nil(err)
	s, err := fake.Core().Services("").List(api.ListOptions{})
	require.Nil(err)
	n, err := fake.Core().Nodes().List(api.ListOptions{})
	require.Nil(err)

	assert.Equal(2, len(m.Items))
	assert.Equal(2, len(s.Items))
	assert.Equal(3, len(n.Items))

	useNodeInternal = false
	ProcessNodeUpdate(n.Items, err)

	// ConfigMap ADDED
	endptStore := newStore(nil)
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfgFoo,
	}, true, endptStore)
	assert.Equal(1, len(virtualServers.m))
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)

	// Second ConfigMap ADDED
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfgBar,
	}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"bar", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)

	// Service ADDED
	ProcessServiceUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		foo}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))

	// Second Service ADDED
	ProcessServiceUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		bar}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))

	// ConfigMap UPDATED
	ProcessConfigMapUpdate(fake, eventStream.Updated, eventStream.ChangedObject{
		cfgFoo,
		cfgFoo,
	}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))

	// Service UPDATED
	ProcessServiceUpdate(fake, eventStream.Updated, eventStream.ChangedObject{
		foo,
		foo}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))

	// ConfigMap ADDED second foo port
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfgFoo8080}, true, endptStore)
	assert.Equal(3, len(virtualServers.m))
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"foo", 8080, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"bar", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)

	// ConfigMap ADDED third foo port
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfgFoo9090}, true, endptStore)
	assert.Equal(4, len(virtualServers.m))
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"foo", 9090, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"foo", 8080, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"bar", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)

	// Nodes ADDED
	_, err = fake.Core().Nodes().Create(extraNode)
	require.Nil(err)
	useNodeInternal = false
	n, err = fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	ProcessNodeUpdate(n.Items, err)
	assert.Equal(4, len(virtualServers.m))
	assert.EqualValues(append(addrs, "127.0.0.3"),
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(append(addrs, "127.0.0.3"),
		virtualServers.m[serviceKey{"bar", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(append(addrs, "127.0.0.3"),
		virtualServers.m[serviceKey{"foo", 8080, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(append(addrs, "127.0.0.3"),
		virtualServers.m[serviceKey{"foo", 9090, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	validateConfig(t, mw, twoSvcsFourPortsThreeNodesConfig)

	// ConfigMap DELETED third foo port
	ProcessConfigMapUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
		cfgFoo9090,
		nil}, true, endptStore)
	assert.Equal(3, len(virtualServers.m))
	assert.NotContains(virtualServers.m, serviceKey{"foo", 9090, "default"},
		"Virtual servers should not contain removed port")
	assert.Contains(virtualServers.m, serviceKey{"foo", 8080, "default"},
		"Virtual servers should contain remaining ports")
	assert.Contains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Virtual servers should contain remaining ports")
	assert.Contains(virtualServers.m, serviceKey{"bar", 80, "default"},
		"Virtual servers should contain remaining ports")

	// ConfigMap UPDATED second foo port
	ProcessConfigMapUpdate(fake, eventStream.Updated, eventStream.ChangedObject{
		cfgFoo8080,
		cfgFoo8080}, true, endptStore)
	assert.Equal(3, len(virtualServers.m))
	assert.Contains(virtualServers.m, serviceKey{"foo", 8080, "default"},
		"Virtual servers should contain remaining ports")
	assert.Contains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Virtual servers should contain remaining ports")
	assert.Contains(virtualServers.m, serviceKey{"bar", 80, "default"},
		"Virtual servers should contain remaining ports")

	// ConfigMap DELETED second foo port
	ProcessConfigMapUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
		cfgFoo8080,
		nil}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))
	assert.Contains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Virtual servers should contain remaining ports")
	assert.Contains(virtualServers.m, serviceKey{"bar", 80, "default"},
		"Virtual servers should contain remaining ports")

	// Nodes DELETES
	err = fake.Core().Nodes().Delete("node1", &api.DeleteOptions{})
	require.Nil(err)
	err = fake.Core().Nodes().Delete("node2", &api.DeleteOptions{})
	require.Nil(err)
	useNodeInternal = false
	n, err = fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	ProcessNodeUpdate(n.Items, err)
	assert.Equal(2, len(virtualServers.m))
	assert.EqualValues([]string{"127.0.0.3"},
		virtualServers.m[serviceKey{"foo", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues([]string{"127.0.0.3"},
		virtualServers.m[serviceKey{"bar", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	validateConfig(t, mw, twoSvcsOneNodeConfig)

	// ConfigMap DELETED
	err = fake.Core().ConfigMaps("default").Delete("foomap", &api.DeleteOptions{})
	m, err = fake.Core().ConfigMaps("").List(api.ListOptions{})
	assert.Equal(1, len(m.Items))
	ProcessConfigMapUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
		cfgFoo,
		nil,
	}, true, endptStore)
	assert.Equal(1, len(virtualServers.m))
	assert.NotContains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Config map should be removed after delete")
	validateConfig(t, mw, oneSvcOneNodeConfig)

	// Service DELETED
	err = fake.Core().Services("default").Delete("bar", &api.DeleteOptions{})
	require.Nil(err)
	s, err = fake.Core().Services("").List(api.ListOptions{})
	assert.Equal(1, len(s.Items))
	ProcessServiceUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
		bar,
		nil}, true, endptStore)
	assert.Equal(1, len(virtualServers.m))
	validateConfig(t, mw, emptyConfig)
}

func TestDontCareConfigMapNodePort(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	assert := assert.New(t)
	require := require.New(t)

	cfg := newConfigMap("foomap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   "bar"})
	svc := newService("foo", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 30001}})

	fake := fake.NewSimpleClientset(&v1.ConfigMapList{Items: []v1.ConfigMap{*cfg}},
		&v1.ServiceList{Items: []v1.Service{*svc}})
	require.NotNil(fake, "Mock client cannot be nil")

	m, err := fake.Core().ConfigMaps("").List(api.ListOptions{})
	require.Nil(err)
	s, err := fake.Core().Services("").List(api.ListOptions{})
	require.Nil(err)

	assert.Equal(1, len(m.Items))
	assert.Equal(1, len(s.Items))

	// ConfigMap ADDED
	assert.Equal(0, len(virtualServers.m))
	endptStore := newStore(nil)
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfg,
	}, true, endptStore)
	assert.Equal(0, len(virtualServers.m))
}

func testConfigMapKeysImpl(t *testing.T, isNodePort bool) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)
	assert := assert.New(t)

	fake := fake.NewSimpleClientset()
	require.NotNil(fake, "Mock client should not be nil")

	noschemakey := newConfigMap("noschema", "1", "default", map[string]string{
		"data": "bar"})
	cfg, err := parseVirtualServerConfig(noschemakey)
	require.Nil(cfg, "Should not have parsed bad configmap")
	require.EqualError(err, "configmap noschema does not contain schema key",
		"Should receive no schema error")
	endptStore := newStore(nil)
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		noschemakey,
	}, isNodePort, endptStore)
	require.Equal(0, len(virtualServers.m))

	nodatakey := newConfigMap("nodata", "1", "default", map[string]string{
		"schema": schemaUrl,
	})
	cfg, err = parseVirtualServerConfig(nodatakey)
	require.Nil(cfg, "Should not have parsed bad configmap")
	require.EqualError(err, "configmap nodata does not contain data key",
		"Should receive no data error")
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		nodatakey,
	}, isNodePort, endptStore)
	require.Equal(0, len(virtualServers.m))

	badjson := newConfigMap("badjson", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   "///// **invalid json** /////",
	})
	cfg, err = parseVirtualServerConfig(badjson)
	require.Nil(cfg, "Should not have parsed bad configmap")
	require.EqualError(err,
		"invalid character '/' looking for beginning of value")
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		badjson,
	}, isNodePort, endptStore)
	require.Equal(0, len(virtualServers.m))

	extrakeys := newConfigMap("extrakeys", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo,
		"key1":   "value1",
		"key2":   "value2",
	})
	cfg, err = parseVirtualServerConfig(extrakeys)
	require.NotNil(cfg, "Config map should parse with extra keys")
	require.Nil(err, "Should not receive errors")
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		extrakeys,
	}, isNodePort, endptStore)
	require.Equal(1, len(virtualServers.m))

	vs, ok := virtualServers.m[serviceKey{"foo", 80, "default"}]
	assert.True(ok, "Config map should be accessible")
	assert.NotNil(vs, "Config map should be object")

	require.Equal("round-robin", vs.VirtualServer.Frontend.Balance)
	require.Equal("http", vs.VirtualServer.Frontend.Mode)
	require.Equal("velcro", vs.VirtualServer.Frontend.Partition)
	require.Equal("10.128.10.240",
		vs.VirtualServer.Frontend.VirtualAddress.BindAddr)
	require.Equal(int32(5051), vs.VirtualServer.Frontend.VirtualAddress.Port)
}

func TestNamespaceIsolation(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)
	assert := assert.New(t)

	fake := fake.NewSimpleClientset()
	require.NotNil(fake, "Mock client should not be nil")

	cfgFoo := newConfigMap("foomap", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgBar := newConfigMap("foomap", "1", "wrongnamespace", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	servFoo := newService("foo", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 37001}})
	servBar := newService("foo", "1", "wrongnamespace", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 50000}})

	endptStore := newStore(nil)
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil, cfgFoo}, true, endptStore)
	_, ok = virtualServers.m[serviceKey{"foo", 80, "default"}]
	assert.True(ok, "Config map should be accessible")

	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil, cfgBar}, true, endptStore)
	_, ok = virtualServers.m[serviceKey{"foo", 80, "wrongnamespace"}]
	assert.False(ok, "Config map should not be added if namespace does not match flag")
	assert.Contains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Virtual servers should contain original config")
	assert.Equal(1, len(virtualServers.m), "There should only be 1 virtual server")

	ProcessConfigMapUpdate(fake, eventStream.Updated, eventStream.ChangedObject{
		cfgBar, cfgBar}, true, endptStore)
	_, ok = virtualServers.m[serviceKey{"foo", 80, "wrongnamespace"}]
	assert.False(ok, "Config map should not be added if namespace does not match flag")
	assert.Contains(virtualServers.m, serviceKey{"foo", 80, "default"},
		"Virtual servers should contain original config")
	assert.Equal(1, len(virtualServers.m), "There should only be 1 virtual server")

	ProcessConfigMapUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
		cfgBar, nil}, true, endptStore)
	_, ok = virtualServers.m[serviceKey{"foo", 80, "wrongnamespace"}]
	assert.False(ok, "Config map should not be deleted if namespace does not match flag")
	_, ok = virtualServers.m[serviceKey{"foo", 80, "default"}]
	assert.True(ok, "Config map should be accessible after delete called on incorrect namespace")

	ProcessServiceUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil, servFoo}, true, endptStore)
	vs, ok := virtualServers.m[serviceKey{"foo", 80, "default"}]
	assert.True(ok, "Service should be accessible")
	assert.EqualValues(37001, vs.VirtualServer.Backend.PoolMemberPort, "Port should match initial config")

	ProcessServiceUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil, servBar}, true, endptStore)
	_, ok = virtualServers.m[serviceKey{"foo", 80, "wrongnamespace"}]
	assert.False(ok, "Service should not be added if namespace does not match flag")
	vs, ok = virtualServers.m[serviceKey{"foo", 80, "default"}]
	assert.True(ok, "Service should be accessible")
	assert.EqualValues(37001, vs.VirtualServer.Backend.PoolMemberPort, "Port should match initial config")

	ProcessServiceUpdate(fake, eventStream.Updated, eventStream.ChangedObject{
		servBar, servBar}, true, endptStore)
	_, ok = virtualServers.m[serviceKey{"foo", 80, "wrongnamespace"}]
	assert.False(ok, "Service should not be added if namespace does not match flag")
	vs, ok = virtualServers.m[serviceKey{"foo", 80, "default"}]
	assert.True(ok, "Service should be accessible")
	assert.EqualValues(37001, vs.VirtualServer.Backend.PoolMemberPort, "Port should match initial config")

	ProcessServiceUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
		servBar, nil}, true, endptStore)
	vs, ok = virtualServers.m[serviceKey{"foo", 80, "default"}]
	assert.True(ok, "Service should not have been deleted")
	assert.EqualValues(37001, vs.VirtualServer.Backend.PoolMemberPort, "Port should match initial config")
}

func TestConfigMapKeysNodePort(t *testing.T) {
	testConfigMapKeysImpl(t, true)
}

func TestConfigMapKeysCluster(t *testing.T) {
	testConfigMapKeysImpl(t, false)
}

func TestProcessUpdatesIAppNodePort(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	assert := assert.New(t)
	require := require.New(t)

	// Create a test env with two ConfigMaps, two Services, and three Nodes
	cfgIapp1 := newConfigMap("iapp1map", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapIApp1})
	cfgIapp2 := newConfigMap("iapp2map", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapIApp2})
	iapp1 := newService("iapp1", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 10101}})
	iapp2 := newService("iapp2", "1", "default", "NodePort",
		[]v1.ServicePort{{Port: 80, NodePort: 20202}})
	nodes := []v1.Node{
		*newNode("node0", "0", true, []v1.NodeAddress{
			{"InternalIP", "192.168.0.0"}}),
		*newNode("node1", "1", false, []v1.NodeAddress{
			{"InternalIP", "192.168.0.1"}}),
		*newNode("node2", "2", false, []v1.NodeAddress{
			{"InternalIP", "192.168.0.2"}}),
		*newNode("node3", "3", false, []v1.NodeAddress{
			{"ExternalIP", "192.168.0.3"}}),
	}
	extraNode := newNode("node4", "4", false, []v1.NodeAddress{{"InternalIP",
		"192.168.0.4"}})

	addrs := []string{"192.168.0.1", "192.168.0.2"}

	fake := fake.NewSimpleClientset(
		&v1.ConfigMapList{Items: []v1.ConfigMap{*cfgIapp1, *cfgIapp2}},
		&v1.ServiceList{Items: []v1.Service{*iapp1, *iapp2}},
		&v1.NodeList{Items: nodes})
	require.NotNil(fake, "Mock client cannot be nil")

	m, err := fake.Core().ConfigMaps("").List(api.ListOptions{})
	require.Nil(err)
	s, err := fake.Core().Services("").List(api.ListOptions{})
	require.Nil(err)
	n, err := fake.Core().Nodes().List(api.ListOptions{})
	require.Nil(err)

	assert.Equal(2, len(m.Items))
	assert.Equal(2, len(s.Items))
	assert.Equal(4, len(n.Items))

	useNodeInternal = true
	ProcessNodeUpdate(n.Items, err)

	// ConfigMap ADDED
	endptStore := newStore(nil)
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfgIapp1,
	}, true, endptStore)
	assert.Equal(1, len(virtualServers.m))
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"iapp1", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)

	// Second ConfigMap ADDED
	ProcessConfigMapUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		cfgIapp2,
	}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"iapp1", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(addrs,
		virtualServers.m[serviceKey{"iapp2", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)

	// Service ADDED
	ProcessServiceUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		iapp1}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))

	// Second Service ADDED
	ProcessServiceUpdate(fake, eventStream.Added, eventStream.ChangedObject{
		nil,
		iapp2}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))

	// ConfigMap UPDATED
	ProcessConfigMapUpdate(fake, eventStream.Updated, eventStream.ChangedObject{
		cfgIapp1,
		cfgIapp1,
	}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))

	// Service UPDATED
	ProcessServiceUpdate(fake, eventStream.Updated, eventStream.ChangedObject{
		iapp1,
		iapp1}, true, endptStore)
	assert.Equal(2, len(virtualServers.m))

	// Nodes ADDED
	_, err = fake.Core().Nodes().Create(extraNode)
	require.Nil(err)
	useNodeInternal = true
	n, err = fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	ProcessNodeUpdate(n.Items, err)
	assert.Equal(2, len(virtualServers.m))
	assert.EqualValues(append(addrs, "192.168.0.4"),
		virtualServers.m[serviceKey{"iapp1", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues(append(addrs, "192.168.0.4"),
		virtualServers.m[serviceKey{"iapp2", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	validateConfig(t, mw, twoIappsThreeNodesConfig)

	// Nodes DELETES
	err = fake.Core().Nodes().Delete("node1", &api.DeleteOptions{})
	require.Nil(err)
	err = fake.Core().Nodes().Delete("node2", &api.DeleteOptions{})
	require.Nil(err)
	useNodeInternal = true
	n, err = fake.Core().Nodes().List(api.ListOptions{})
	assert.Nil(err, "Should not fail listing nodes")
	ProcessNodeUpdate(n.Items, err)
	assert.Equal(2, len(virtualServers.m))
	assert.EqualValues([]string{"192.168.0.4"},
		virtualServers.m[serviceKey{"iapp1", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	assert.EqualValues([]string{"192.168.0.4"},
		virtualServers.m[serviceKey{"iapp2", 80, "default"}].VirtualServer.Backend.PoolMemberAddrs)
	validateConfig(t, mw, twoIappsOneNodeConfig)

	// ConfigMap DELETED
	err = fake.Core().ConfigMaps("default").Delete("iapp1map",
		&api.DeleteOptions{})
	m, err = fake.Core().ConfigMaps("").List(api.ListOptions{})
	assert.Equal(1, len(m.Items))
	ProcessConfigMapUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
		cfgIapp1,
		nil,
	}, true, endptStore)
	assert.Equal(1, len(virtualServers.m))
	assert.NotContains(virtualServers.m, serviceKey{"iapp1", 80, "default"},
		"Config map should be removed after delete")
	validateConfig(t, mw, oneIappOneNodeConfig)

	// Service DELETED
	err = fake.Core().Services("default").Delete("iapp2", &api.DeleteOptions{})
	require.Nil(err)
	s, err = fake.Core().Services("").List(api.ListOptions{})
	assert.Equal(1, len(s.Items))
	ProcessServiceUpdate(fake, eventStream.Deleted, eventStream.ChangedObject{
		iapp2,
		nil}, true, endptStore)
	assert.Equal(1, len(virtualServers.m))
	validateConfig(t, mw, emptyConfig)
}

func TestSchemaValidation(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)
	assert := assert.New(t)

	fake := fake.NewSimpleClientset()
	require.NotNil(fake, "Mock client should not be nil")

	// JSON is valid, but values are invalid
	var configmapFoo string = string(`{
	  "virtualServer": {
	    "backend": {
	      "serviceName": "",
	      "servicePort": 0
	    },
	    "frontend": {
	      "balance": "super-duper-mojo",
	      "mode": "udp",
	      "partition": "",
	      "virtualAddress": {
	        "bindAddr": "10.128.10.260",
	        "port": 500000
	      },
	      "sslProfile": {
	        "f5ProfileName": ""
	      }
	    }
	  }
	}`)

	badjson := newConfigMap("badjson", "1", "default", map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo,
	})
	cfg, err := parseVirtualServerConfig(badjson)
	require.Nil(cfg, "Should not have parsed bad configmap")
	assert.Contains(err.Error(),
		"virtualServer.frontend.partition: String length must be greater than or equal to 1")
	assert.Contains(err.Error(),
		"virtualServer.frontend.mode: virtualServer.frontend.mode must be one of the following: \\\"http\\\", \\\"tcp\\\"")
	assert.Contains(err.Error(),
		"virtualServer.frontend.balance: virtualServer.frontend.balance must be one of the following:")
	assert.Contains(err.Error(),
		"virtualServer.frontend.sslProfile.f5ProfileName: String length must be greater than or equal to 1")
	assert.Contains(err.Error(),
		"virtualServer.frontend.virtualAddress.bindAddr: Does not match format 'ipv4'")
	assert.Contains(err.Error(),
		"virtualServer.frontend.virtualAddress.port: Must be less than or equal to 65535")
	assert.Contains(err.Error(),
		"virtualServer.backend.serviceName: String length must be greater than or equal to 1")
	assert.Contains(err.Error(),
		"virtualServer.backend.servicePort: Must be greater than or equal to 1")
}

func validateServiceIps(t *testing.T, serviceName, namespace string,
	svcPorts []v1.ServicePort, ips []string) {
	for _, p := range svcPorts {
		vs, ok := virtualServers.m[serviceKey{serviceName, p.Port, namespace}]
		require.True(t, ok)
		require.NotNil(t, vs)
		var expectedIps []string
		if ips != nil {
			expectedIps = []string{}
			for _, ip := range ips {
				ip = ip + ":" + strconv.Itoa(int(p.Port))
				expectedIps = append(expectedIps, ip)
			}
		}
		require.EqualValues(t, expectedIps, vs.VirtualServer.Backend.PoolMemberAddrs,
			"nodes are not correct")
	}
}

func TestVirtualServerWhenEndpointsEmpty(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)

	svcName := "foo"
	emptyIps := []string{}
	readyIps := []string{"10.2.96.0", "10.2.96.1", "10.2.96.2"}
	notReadyIps := []string{"10.2.96.3", "10.2.96.4", "10.2.96.5", "10.2.96.6"}
	svcPorts := []v1.ServicePort{
		newServicePort("port0", 80),
	}

	cfgFoo := newConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})

	foo := newService(svcName, "1", namespace, v1.ServiceTypeClusterIP, svcPorts)
	fake := fake.NewSimpleClientset(&v1.ServiceList{Items: []v1.Service{*foo}})

	svcStore := newStore(nil)
	svcStore.Add(foo)
	var endptStore *eventStream.EventStore
	onEndptChange := func(changeType eventStream.ChangeType, obj interface{}) {
		ProcessEndpointsUpdate(fake, changeType, obj, svcStore)
	}
	endptStore = newStore(onEndptChange)

	endptPorts := convertSvcPortsToEndpointPorts(svcPorts)
	goodEndpts := newEndpoints(svcName, "1", namespace, emptyIps, emptyIps,
		endptPorts)

	err := endptStore.Add(goodEndpts)
	require.Nil(err)
	// this is for another service
	badEndpts := newEndpoints("wrongSvc", "1", namespace, []string{"10.2.96.7"},
		[]string{}, endptPorts)
	err = endptStore.Add(badEndpts)
	require.Nil(err)

	r := processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil, cfgFoo}, false, endptStore)
	require.True(r, "Config map should be processed")

	require.Equal(len(svcPorts), len(virtualServers.m))
	for _, p := range svcPorts {
		require.Contains(virtualServers.m, serviceKey{"foo", p.Port, namespace})
		vs := virtualServers.m[serviceKey{"foo", p.Port, namespace}]
		require.EqualValues(0, vs.VirtualServer.Backend.PoolMemberPort)
	}

	validateServiceIps(t, svcName, namespace, svcPorts, []string{})

	// Move it back to ready from not ready and make sure it is re-added
	err = endptStore.Update(newEndpoints(svcName, "2", namespace, readyIps,
		notReadyIps, endptPorts))
	require.Nil(err)
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps)

	// Remove all endpoints make sure they are removed but virtual server exists
	err = endptStore.Update(newEndpoints(svcName, "3", namespace, emptyIps,
		emptyIps, endptPorts))
	require.Nil(err)
	validateServiceIps(t, svcName, namespace, svcPorts, []string{})

	// Move it back to ready from not ready and make sure it is re-added
	err = endptStore.Update(newEndpoints(svcName, "4", namespace, readyIps,
		notReadyIps, endptPorts))
	require.Nil(err)
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps)
}

func TestVirtualServerWhenEndpointsChange(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)

	svcName := "foo"
	emptyIps := []string{}
	readyIps := []string{"10.2.96.0", "10.2.96.1", "10.2.96.2"}
	notReadyIps := []string{"10.2.96.3", "10.2.96.4", "10.2.96.5", "10.2.96.6"}
	svcPorts := []v1.ServicePort{
		newServicePort("port0", 80),
		newServicePort("port1", 8080),
		newServicePort("port2", 9090),
	}

	cfgFoo := newConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgFoo8080 := newConfigMap("foomap8080", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgFoo9090 := newConfigMap("foomap9090", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo9090})

	foo := newService(svcName, "1", namespace, v1.ServiceTypeClusterIP, svcPorts)
	fake := fake.NewSimpleClientset(&v1.ServiceList{Items: []v1.Service{*foo}})

	svcStore := newStore(nil)
	svcStore.Add(foo)
	var endptStore *eventStream.EventStore
	onEndptChange := func(changeType eventStream.ChangeType, obj interface{}) {
		ProcessEndpointsUpdate(fake, changeType, obj, svcStore)
	}
	endptStore = newStore(onEndptChange)

	r := processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil, cfgFoo}, false, endptStore)
	require.True(r, "Config map should be processed")

	r = processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil, cfgFoo8080}, false, endptStore)
	require.True(r, "Config map should be processed")

	r = processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil, cfgFoo9090}, false, endptStore)
	require.True(r, "Config map should be processed")

	require.Equal(len(svcPorts), len(virtualServers.m))
	for _, p := range svcPorts {
		require.Contains(virtualServers.m, serviceKey{"foo", p.Port, namespace})
	}

	endptPorts := convertSvcPortsToEndpointPorts(svcPorts)
	goodEndpts := newEndpoints(svcName, "1", namespace, readyIps, notReadyIps,
		endptPorts)
	err := endptStore.Add(goodEndpts)
	require.Nil(err)
	// this is for another service
	badEndpts := newEndpoints("wrongSvc", "1", namespace, []string{"10.2.96.7"},
		[]string{}, endptPorts)
	err = endptStore.Add(badEndpts)
	require.Nil(err)

	validateServiceIps(t, svcName, namespace, svcPorts, readyIps)

	// Move an endpoint from ready to not ready and make sure it
	// goes away from virtual servers
	notReadyIps = append(notReadyIps, readyIps[len(readyIps)-1])
	readyIps = readyIps[:len(readyIps)-1]
	err = endptStore.Update(newEndpoints(svcName, "2", namespace, readyIps,
		notReadyIps, endptPorts))
	require.Nil(err)
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps)

	// Move it back to ready from not ready and make sure it is re-added
	readyIps = append(readyIps, notReadyIps[len(notReadyIps)-1])
	notReadyIps = notReadyIps[:len(notReadyIps)-1]
	err = endptStore.Update(newEndpoints(svcName, "3", namespace, readyIps,
		notReadyIps, endptPorts))
	require.Nil(err)
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps)

	// Remove all endpoints make sure they are removed but virtual server exists
	err = endptStore.Update(newEndpoints(svcName, "4", namespace, emptyIps,
		emptyIps, endptPorts))
	require.Nil(err)
	validateServiceIps(t, svcName, namespace, svcPorts, []string{})

	// Move it back to ready from not ready and make sure it is re-added
	err = endptStore.Update(newEndpoints(svcName, "5", namespace, readyIps,
		notReadyIps, endptPorts))
	require.Nil(err)
	validateServiceIps(t, svcName, namespace, svcPorts, readyIps)
}

func TestVirtualServerWhenServiceChanges(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)

	svcName := "foo"
	svcPorts := []v1.ServicePort{
		newServicePort("port0", 80),
		newServicePort("port1", 8080),
		newServicePort("port2", 9090),
	}
	svcPodIps := []string{"10.2.96.0", "10.2.96.1", "10.2.96.2"}
	endptStore := newStore(nil)

	foo := newService(svcName, "1", namespace, v1.ServiceTypeClusterIP, svcPorts)
	fake := fake.NewSimpleClientset(&v1.ServiceList{Items: []v1.Service{*foo}})

	onSvcChange := func(changeType eventStream.ChangeType, obj interface{}) {
		if changeType == eventStream.Replaced {
			v := obj.([]interface{})
			for _, item := range v {
				processService(fake, changeType, item, false, endptStore)
			}
		} else {
			processService(fake, changeType, obj, false, endptStore)
		}
	}
	svcStore := newStore(onSvcChange)
	svcStore.Add(foo)

	endptPorts := convertSvcPortsToEndpointPorts(svcPorts)
	err := endptStore.Add(newEndpoints(svcName, "1", namespace, svcPodIps,
		[]string{}, endptPorts))
	require.Nil(err)

	cfgFoo := newConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgFoo8080 := newConfigMap("foomap8080", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgFoo9090 := newConfigMap("foomap9090", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo9090})

	r := processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil, cfgFoo}, false, endptStore)
	require.True(r, "Config map should be processed")

	r = processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil, cfgFoo8080}, false, endptStore)
	require.True(r, "Config map should be processed")

	r = processConfigMap(fake, eventStream.Added, eventStream.ChangedObject{
		nil, cfgFoo9090}, false, endptStore)
	require.True(r, "Config map should be processed")

	require.Equal(len(svcPorts), len(virtualServers.m))
	validateServiceIps(t, svcName, namespace, svcPorts, svcPodIps)

	// delete the service and make sure the IPs go away on the VS
	svcStore.Delete(foo)
	require.Equal(len(svcPorts), len(virtualServers.m))
	validateServiceIps(t, svcName, namespace, svcPorts, nil)

	// re-add the service
	foo.ObjectMeta.ResourceVersion = "2"
	svcStore.Add(foo)
	require.Equal(len(svcPorts), len(virtualServers.m))
	validateServiceIps(t, svcName, namespace, svcPorts, svcPodIps)
}

func TestVirtualServerWhenConfigMapChanges(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	require := require.New(t)

	svcName := "foo"
	svcPorts := []v1.ServicePort{
		newServicePort("port0", 80),
		newServicePort("port1", 8080),
		newServicePort("port2", 9090),
	}
	svcPodIps := []string{"10.2.96.0", "10.2.96.1", "10.2.96.2"}
	endptStore := newStore(nil)
	endptPorts := convertSvcPortsToEndpointPorts(svcPorts)
	err := endptStore.Add(newEndpoints(svcName, "1", namespace, svcPodIps,
		[]string{}, endptPorts))
	require.Nil(err)

	foo := newService(svcName, "1", namespace, v1.ServiceTypeClusterIP, svcPorts)
	fake := fake.NewSimpleClientset(&v1.ServiceList{Items: []v1.Service{*foo}})

	// no virtual servers yet
	require.Equal(0, len(virtualServers.m))

	onCfgChange := func(changeType eventStream.ChangeType, obj interface{}) {
		if changeType == eventStream.Replaced {
			v := obj.([]interface{})
			for _, item := range v {
				processConfigMap(fake, changeType, item, false, endptStore)
			}
		} else {
			processConfigMap(fake, changeType, obj, false, endptStore)
		}
	}
	cfgStore := newStore(onCfgChange)

	// add a config map
	cfgFoo := newConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo})
	cfgStore.Add(cfgFoo)
	require.Equal(1, len(virtualServers.m))
	validateServiceIps(t, svcName, namespace, svcPorts[:1], svcPodIps)

	// add another
	cfgFoo8080 := newConfigMap("foomap8080", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgStore.Add(cfgFoo8080)
	require.Equal(2, len(virtualServers.m))
	validateServiceIps(t, svcName, namespace, svcPorts[:2], svcPodIps)

	// remove first one
	cfgStore.Delete(cfgFoo)
	require.Equal(1, len(virtualServers.m))
	validateServiceIps(t, svcName, namespace, svcPorts[1:2], svcPodIps)
}

func TestUpdatesConcurrentCluster(t *testing.T) {
	config = &test.MockWriter{
		FailStyle: test.Success,
		Sections:  make(map[string]interface{}),
	}
	mw, ok := config.(*test.MockWriter)
	assert.NotNil(t, mw)
	assert.True(t, ok)

	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	assert := assert.New(t)
	require := require.New(t)

	fooIps := []string{"10.2.96.1", "10.2.96.2"}
	fooPorts := []v1.ServicePort{newServicePort("port0", 8080)}
	barIps := []string{"10.2.96.0", "10.2.96.3"}
	barPorts := []v1.ServicePort{newServicePort("port1", 80)}

	cfgFoo := newConfigMap("foomap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapFoo8080})
	cfgBar := newConfigMap("barmap", "1", namespace, map[string]string{
		"schema": schemaUrl,
		"data":   configmapBar})

	foo := newService("foo", "1", namespace, v1.ServiceTypeClusterIP, fooPorts)
	bar := newService("bar", "1", namespace, v1.ServiceTypeClusterIP, barPorts)

	fake := fake.NewSimpleClientset()
	require.NotNil(fake, "Mock client cannot be nil")

	var cfgStore *eventStream.EventStore
	var endptStore *eventStream.EventStore
	var svcStore *eventStream.EventStore

	onCfgChange := func(changeType eventStream.ChangeType, obj interface{}) {
		ProcessConfigMapUpdate(fake, changeType, obj, false, endptStore)
	}
	cfgStore = newStore(onCfgChange)

	onEndptChange := func(changeType eventStream.ChangeType, obj interface{}) {
		ProcessEndpointsUpdate(fake, changeType, obj, svcStore)
	}
	endptStore = newStore(onEndptChange)

	onSvcChange := func(changeType eventStream.ChangeType, obj interface{}) {
		ProcessServiceUpdate(fake, changeType, obj, false, endptStore)
		o, ok := obj.(eventStream.ChangedObject)
		require.True(ok, "expected eventStream.ChangedObject")
		switch changeType {
		case eventStream.Added:
			svc := o.New.(*v1.Service)
			fSvc, err := fake.Core().Services(namespace).Create(svc)
			require.Nil(err, "Should not fail creating service")
			require.EqualValues(fSvc, svc, "Service should be equal")
		case eventStream.Deleted:
			svc := o.Old.(*v1.Service)
			err := fake.Core().Services(namespace).Delete(svc.ObjectMeta.Name,
				&api.DeleteOptions{})
			require.Nil(err, "Should not error deleting service")
		}
	}
	svcStore = newStore(onSvcChange)

	fooEndpts := newEndpoints("foo", "1", namespace, fooIps, barIps,
		convertSvcPortsToEndpointPorts(fooPorts))
	barEndpts := newEndpoints("bar", "1", namespace, barIps, fooIps,
		convertSvcPortsToEndpointPorts(barPorts))
	cfgCh := make(chan struct{})
	endptCh := make(chan struct{})
	svcCh := make(chan struct{})

	go func() {
		err := endptStore.Add(fooEndpts)
		require.Nil(err)
		err = endptStore.Add(barEndpts)
		require.Nil(err)

		endptCh <- struct{}{}
	}()

	go func() {
		err := cfgStore.Add(cfgFoo)
		require.Nil(err, "Should not fail creating configmap")

		err = cfgStore.Add(cfgBar)
		require.Nil(err, "Should not fail creating configmap")

		cfgCh <- struct{}{}
	}()

	go func() {
		err := svcStore.Add(foo)
		require.Nil(err, "Should not fail creating service")

		err = svcStore.Add(bar)
		require.Nil(err, "Should not fail creating service")

		svcCh <- struct{}{}
	}()

	select {
	case <-endptCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting endpoints channel notification")
	}
	select {
	case <-cfgCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting configmap channel notification")
	}
	select {
	case <-svcCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out excpecting service channel notification")
	}

	validateConfig(t, mw, twoSvcTwoPodsConfig)

	go func() {
		// delete endpoints for foo
		err := endptStore.Delete(fooEndpts)
		require.Nil(err)

		endptCh <- struct{}{}
	}()

	go func() {
		// delete cfgmap for foo
		err := cfgStore.Delete(cfgFoo)
		require.Nil(err, "Should not error deleting map")

		cfgCh <- struct{}{}
	}()

	go func() {
		// Delete service for foo
		err := svcStore.Delete(foo)
		require.Nil(err, "Should not error deleting service")

		svcCh <- struct{}{}
	}()

	select {
	case <-endptCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting endpoints channel notification")
	}
	select {
	case <-cfgCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out expecting configmap channel notification")
	}
	select {
	case <-svcCh:
	case <-time.After(time.Second * 30):
		assert.FailNow("Timed out excpecting service channel notification")
	}
	assert.Equal(1, len(virtualServers.m))
	validateConfig(t, mw, oneSvcTwoPodsConfig)
}

func TestNonNodePortServiceModeNodePort(t *testing.T) {
	defer func() {
		virtualServers.m = make(map[serviceKey]*VirtualServerConfig)
	}()

	assert := assert.New(t)
	require := require.New(t)

	cfgFoo := newConfigMap(
		"foomap",
		"1",
		"default",
		map[string]string{
			"schema": schemaUrl,
			"data":   configmapFoo,
		},
	)

	fake := fake.NewSimpleClientset()
	require.NotNil(fake, "Mock client cannot be nil")

	endptStore := newStore(nil)
	r := processConfigMap(
		fake,
		eventStream.Added,
		eventStream.ChangedObject{nil, cfgFoo},
		true,
		endptStore,
	)
	require.True(r, "Config map should be processed")

	require.Equal(1, len(virtualServers.m))
	require.Contains(
		virtualServers.m,
		serviceKey{"foo", 80, "default"},
		"Virtual servers should have an entry",
	)

	foo := newService(
		"foo",
		"1",
		"default",
		"ClusterIP",
		[]v1.ServicePort{{Port: 80}},
	)

	r = processService(
		fake,
		eventStream.Added,
		eventStream.ChangedObject{nil, foo},
		true,
		endptStore,
	)

	assert.False(r, "Should not process non NodePort Service")
}
