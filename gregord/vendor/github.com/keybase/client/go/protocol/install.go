// Auto-generated by avdl-compiler v1.3.1 (https://github.com/keybase/node-avdl-compiler)
//   Input file: avdl/install.avdl

package keybase1

import (
	rpc "github.com/keybase/go-framed-msgpack-rpc"
)

// Install status describes state of install for a component or service.
type InstallStatus int

const (
	InstallStatus_UNKNOWN       InstallStatus = 0
	InstallStatus_ERROR         InstallStatus = 1
	InstallStatus_NOT_INSTALLED InstallStatus = 2
	InstallStatus_INSTALLED     InstallStatus = 4
)

type InstallAction int

const (
	InstallAction_UNKNOWN   InstallAction = 0
	InstallAction_NONE      InstallAction = 1
	InstallAction_UPGRADE   InstallAction = 2
	InstallAction_REINSTALL InstallAction = 3
	InstallAction_INSTALL   InstallAction = 4
)

type ServiceStatus struct {
	Version        string        `codec:"version" json:"version"`
	Label          string        `codec:"label" json:"label"`
	Pid            string        `codec:"pid" json:"pid"`
	LastExitStatus string        `codec:"lastExitStatus" json:"lastExitStatus"`
	BundleVersion  string        `codec:"bundleVersion" json:"bundleVersion"`
	InstallStatus  InstallStatus `codec:"installStatus" json:"installStatus"`
	InstallAction  InstallAction `codec:"installAction" json:"installAction"`
	Status         Status        `codec:"status" json:"status"`
}

type ServicesStatus struct {
	Service []ServiceStatus `codec:"service" json:"service"`
	Kbfs    []ServiceStatus `codec:"kbfs" json:"kbfs"`
}

type FuseMountInfo struct {
	Path   string `codec:"path" json:"path"`
	Fstype string `codec:"fstype" json:"fstype"`
	Output string `codec:"output" json:"output"`
}

type FuseStatus struct {
	Version       string          `codec:"version" json:"version"`
	BundleVersion string          `codec:"bundleVersion" json:"bundleVersion"`
	KextID        string          `codec:"kextID" json:"kextID"`
	Path          string          `codec:"path" json:"path"`
	KextStarted   bool            `codec:"kextStarted" json:"kextStarted"`
	InstallStatus InstallStatus   `codec:"installStatus" json:"installStatus"`
	InstallAction InstallAction   `codec:"installAction" json:"installAction"`
	MountInfos    []FuseMountInfo `codec:"mountInfos" json:"mountInfos"`
	Status        Status          `codec:"status" json:"status"`
}

type ComponentResult struct {
	Name   string `codec:"name" json:"name"`
	Status Status `codec:"status" json:"status"`
}

type InstallResult struct {
	ComponentResults []ComponentResult `codec:"componentResults" json:"componentResults"`
	Status           Status            `codec:"status" json:"status"`
	Fatal            bool              `codec:"fatal" json:"fatal"`
}

type UninstallResult struct {
	ComponentResults []ComponentResult `codec:"componentResults" json:"componentResults"`
	Status           Status            `codec:"status" json:"status"`
}

type InstallInterface interface {
}

func InstallProtocol(i InstallInterface) rpc.Protocol {
	return rpc.Protocol{
		Name:    "keybase.1.install",
		Methods: map[string]rpc.ServeHandlerDescription{},
	}
}

type InstallClient struct {
	Cli rpc.GenericClient
}
