package fipcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	"github.com/hetznercloud/hcloud-go/hcloud"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const Version = "0.0.1"

type Configuration struct {
	Token   string
	Address string
}

type Client struct {
	HetznerClient *hcloud.Client
	KubeClient    *kubernetes.Clientset
	Configuration Configuration
}

func NewClient() (*Client, error) {
	// Move config reading out of NewClient() and pass as struct
	file, err := ioutil.ReadFile("config/config.json")
	if err != nil {
		return nil, fmt.Errorf("could not open config file: %v", err)
	}

	var config Configuration
	err = json.Unmarshal(file, &config)
	if err != nil {
		return nil, fmt.Errorf("could not decode config: %v", err)
	}

	hetznerClient := hcloud.NewClient(hcloud.WithToken(config.Token))

	kubeconfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("could not get kubeconfig: %v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("could not get kubernetes client: %v", err)
	}

	return &Client{
		HetznerClient: hetznerClient,
		KubeClient:    kubeClient,
		Configuration: config,
	}, nil
}

func (client *Client) Run(ctx context.Context) error {
	// TODO: Passing ctx is already great, next you could do a select{} with ctx.Done to gracefully shutdown
	for {
		nodeAddress, err := client.nodeAddress()
		if err != nil {
			return fmt.Errorf("could not get kubernetes node address: %v", err)
		}

		serverAddress, err := client.publicAddress(ctx, nodeAddress)
		if err != nil {
			return fmt.Errorf("could not get current serverAddress: %v", err)
		}

		floatingIP, err := client.floatingIP(ctx)
		if err != nil {
			return err
		}

		if serverAddress.ID != floatingIP.Server.ID {
			fmt.Printf("Switching address %s to serverAddress %s.", floatingIP.IP.String(), serverAddress.Name)
			// TODO: Check if FloatingIP.Assign error returns != 200 OK errors
			// I believe you should check the returned response as the returned error only returns if http call fails
			_, _, err := client.HetznerClient.FloatingIP.Assign(ctx, floatingIP, serverAddress)
			if err != nil {
				return fmt.Errorf("could not update floating IP: %v", err)
			}
		} else {
			fmt.Printf("Address %s already assigned to serverAddress %s. Nothing to do.", floatingIP.IP.String(), serverAddress.Name)
		}

		time.Sleep(30 * time.Second)
	}
}

func (client *Client) floatingIP(ctx context.Context) (ip *hcloud.FloatingIP, err error) {
	ips, err := client.HetznerClient.FloatingIP.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not fetch floating IPs: %v", err)
	}

	for _, ip := range ips {
		if ip.IP.Equal(net.ParseIP(client.Configuration.Address)) {
			return ip, nil
		}
	}

	return nil, fmt.Errorf("IP address %s not allocated", client.Configuration.Address)
}

func (client *Client) publicAddress(ctx context.Context, ip net.IP) (server *hcloud.Server, err error) {
	servers, err := client.HetznerClient.Server.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not fetch servers: %v", err)
	}

	for _, server := range servers {
		if server.PublicNet.IPv4.IP.Equal(ip) {
			return server, nil
		}
	}
	return nil, fmt.Errorf("no server with IP address %s found", ip.String())
}

func (client *Client) nodeAddress() (address net.IP, err error) {
	nodeName := os.Getenv("NODE_NAME")
	nodes, err := client.KubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not list nodes: %v", err)
	}

	var addresses []corev1.NodeAddress
	for _, node := range nodes.Items {
		if node.Name == nodeName {
			addresses = node.Status.Addresses
			break
		}
	}

	for _, address := range addresses {
		if address.Type == corev1.NodeInternalIP {
			return net.ParseIP(address.Address), nil
		}
	}
	return nil, fmt.Errorf("could not find address for current node")
}
