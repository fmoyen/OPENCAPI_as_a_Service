// Copyright 2018 Xilinx Corporation. All Rights Reserved.
// Author: Brian Xu(brianx@xilinx.com)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

//#################################################################################################################################
const (
	SysfsDevices = "/sys/bus/pci/devices"
	// OCP-CAPI-changes
	CXLDevDir  = "/dev/cxl"
	CXLPrefix1 = "afu"
	CXLPrefix2 = ".0m"
	CXLCardSTR = "card"
	// end of OCP-CAPI-changes
	MgmtPrefix     = "/dev/xclmgmt"
	UserPrefix     = "/dev/dri"
	QdmaPrefix     = "/dev/xfpga"
	OcxlPrefix1    = "ocxlfn."
	OcxlPrefix2    = "ocxl"
	QDMASTR        = "dma.qdma.u"
	UserPFKeyword  = "drm"
	DRMSTR         = "renderD"
	ROMSTR         = "rom"
	DSAverFile     = "VBNV"
	DSAtsFile      = "timestamp"
	BoardNameFile  = "board_name"
	ImageLoadFile  = "image_loaded"
	InstanceFile   = "instance"
	MgmtFile       = "mgmt_pf"
	UserFile       = "user_pf"
	VendorFile     = "vendor"
	DeviceFile     = "device"
	SubDeviceFile  = "subsystem_device"
	XilinxVendorID = "0x10ee"
	IBMVendorID    = "0x1014"
	ADVANTECH_ID   = "0x13fe"
	AWS_ID         = "0x1d0f"
	AristaVendorID = "0x3475"
	CAPI2_P_ID     = "0x0632"
	CAPI2_V_ID     = "0x0477"
	OpencapiID     = "0x062b"
	CAPI2_U200     = "0x0665"
	CAPI2_9H3      = "0x0667"
	CAPI2_9H7      = "0x0668"
	OCAPI_9H7      = "0x0666"
)

type Pairs struct {
	Mgmt string
	User string
	Qdma string
}

type Device struct {
	index         string
	shellVer      string
	timestamp     string
	DBDF          string // this is for user pf
	deviceID      string //devid of the user pf
	Healthy       string
	Nodes         *Pairs
	CXLDevAFUPath string // OCP-CAPI-changes
}

//#################################################################################################################################
func GetInstance(DBDF string) (string, error) {
	strArray := strings.Split(DBDF, ":")
	domain, err := strconv.ParseUint(strArray[0], 16, 16)
	if err != nil {
		return "", fmt.Errorf("strconv failed: %s\n", strArray[0])
	}
	bus, err := strconv.ParseUint(strArray[1], 16, 8)
	if err != nil {
		return "", fmt.Errorf("strconv failed: %s\n", strArray[1])
	}
	strArray = strings.Split(strArray[2], ".")
	dev, err := strconv.ParseUint(strArray[0], 16, 8)
	if err != nil {
		return "", fmt.Errorf("strconv failed: %s\n", strArray[0])
	}
	fc, err := strconv.ParseUint(strArray[1], 16, 8)
	if err != nil {
		return "", fmt.Errorf("strconv failed: %s\n", strArray[1])
	}
	ret := domain*65536 + bus*256 + dev*8 + fc
	return strconv.FormatUint(ret, 10), nil
}

//#################################################################################################################################
func GetFileNameFromPrefix(dir string, prefix string) (string, error) {
	userFiles, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("Can't read folder %s", dir)
	}
	for _, userFile := range userFiles {
		fname := userFile.Name()

		if !strings.HasPrefix(fname, prefix) {
			continue
		}
		return fname, nil
	}
	return "", nil
}

//#################################################################################################################################
func GetFileContent(file string) (string, error) {
	if buf, err := ioutil.ReadFile(file); err != nil {
		return "", fmt.Errorf("Can't read file %s", file)
	} else {
		ret := strings.Trim(string(buf), "\n")
		return ret, nil
	}
}

//#################################################################################################################################
//Prior to 2018.3 release, Xilinx FPGA has mgmt PF as func 1 and user PF
//as func 0. The func numbers of the 2 PFs are swapped after 2018.3 release.
//The FPGA device driver in (and after) 2018.3 release creates sysfs file --
//mgmt_pf and user_pf accordingly to reflect what a PF really is.
//
//The plugin will rely on this info to determine whether the a entry is mgmtPF,
//userPF, or none. This also means, it will not support 2018.2 any more.
func FileExist(fname string) bool {
	if _, err := os.Stat(fname); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

//#################################################################################################################################
func IsMgmtPf(pciID string) bool {
	fname := path.Join(SysfsDevices, pciID, MgmtFile)
	return FileExist(fname)
}

//#################################################################################################################################
func IsUserPf(pciID string) bool {
	fname := path.Join(SysfsDevices, pciID, UserFile)
	return FileExist(fname)
}

//#################################################################################################################################
func GetDevices() ([]Device, error) {
	var devices []Device
	pairMap := make(map[string]*Pairs)

	// get the names of the files included in the SysfsDevices directory (such as /sys/bus/pci/devices/0003:01:00.0)
	pciFiles, err := ioutil.ReadDir(SysfsDevices)
	if err != nil {
		return nil, fmt.Errorf("Can't read folder %s", SysfsDevices)
	}

	//=============================================================================================================================
	for _, pciFile := range pciFiles {
		pciID := pciFile.Name()

		// Get the vendor ID of the card
		fname := path.Join(SysfsDevices, pciID, VendorFile)
		vendorID, err := GetFileContent(fname)
		if err != nil {
			return nil, err
		}

		// If card vendorID is not IBMVendorID, then do not get device (do nothing) for this device
		// (future improvement: build a better filter to get only CAPI/OpenCAPI cards)
		if strings.EqualFold(vendorID, IBMVendorID) != true {
			continue
		}

		// if pciID = "0003:01:00.0", DBD = "0003:01:00" (removing the last 2 caracters)
		DBD := pciID[:len(pciID)-2]
		// if pairMap[DBD] does not exist, create it empty
		if _, ok := pairMap[DBD]; !ok {
			pairMap[DBD] = &Pairs{
				Mgmt: "",
				User: "",
				Qdma: "",
			}
		}

		// For containers deployed on top of baremetal machines, xilinx FPGA
		// in sysfs will always appear as pair of mgmt PF and user PF
		// For containers deployed on top of VM, there may be only user PF
		// available(mgmt PF is not assigned to the VM)
		// so mgmt in Pair may be empty

		//-------------------------------------------------------------------------------------------------------------------------
		// user_pf never occurs if CAPI/OpenCAPI Device
		if IsUserPf(pciID) { //user pf
			userDBDF := pciID
			romFolder, err := GetFileNameFromPrefix(path.Join(SysfsDevices, pciID), ROMSTR)
			count := 0
			if err != nil {
				return nil, err
			}
			for romFolder == "" {
				if count >= 3 {
					break
				}
				time.Sleep(3 * time.Second)
				romFolder, err := GetFileNameFromPrefix(path.Join(SysfsDevices, pciID), ROMSTR)
				fmt.Println(romFolder, err)
				count += 1
			}
			// get dsa version
			fname = path.Join(SysfsDevices, pciID, romFolder, DSAverFile)
			content, err := GetFileContent(fname)
			if err != nil {
				return nil, err
			}
			dsaVer := content
			// get dsa timestamp
			fname = path.Join(SysfsDevices, pciID, romFolder, DSAtsFile)
			content, err = GetFileContent(fname)
			if err != nil {
				return nil, err
			}
			dsaTs := content
			// get device id
			fname = path.Join(SysfsDevices, pciID, DeviceFile)
			content, err = GetFileContent(fname)
			if err != nil {
				return nil, err
			}
			devid := content
			// get user PF node
			userpf, err := GetFileNameFromPrefix(path.Join(SysfsDevices, pciID, UserPFKeyword), DRMSTR)
			if err != nil {
				return nil, err
			}
			userNode := path.Join(UserPrefix, userpf)
			pairMap[DBD].User = userNode

			//get qdma device node if it exists
			instance, err := GetInstance(userDBDF)
			if err != nil {
				return nil, err
			}

			qdmaFolder, err := GetFileNameFromPrefix(path.Join(SysfsDevices, pciID), QDMASTR)
			if err != nil {
				return nil, err
			}

			if qdmaFolder != "" {
				pairMap[DBD].Qdma = path.Join(QdmaPrefix, QDMASTR+instance+".0")
			}

			//TODO: check temp, power, fan speed etc, to give a healthy level
			//so far, return Healthy
			healthy := pluginapi.Healthy
			devices = append(devices, Device{
				index:         strconv.Itoa(len(devices) + 1),
				shellVer:      dsaVer,
				timestamp:     dsaTs,
				DBDF:          userDBDF,
				deviceID:      devid,
				Healthy:       healthy,
				Nodes:         pairMap[DBD],
				CXLDevAFUPath: "", // OCP-CAPI-changes
			})
		}
		
		//-------------------------------------------------------------------------------------------------------------------------
		// mgmt_pf never occurs if CAPI/OpenCAPI Device
		// unless if XRT (Xilinx Runtime) is installed (then CAPI2 device may have a mgmt_pf file)
		else if IsMgmtPf(pciID) { //mgmt pf
			// get mgmt instance
			fname = path.Join(SysfsDevices, pciID, InstanceFile)
			content, err := GetFileContent(fname)
			if err != nil {
				return nil, err
			}
			pairMap[DBD].Mgmt = MgmtPrefix + content
		}
		
		//-------------------------------------------------------------------------------------------------------------------------
		// CAPI2 mode virtual slot or OpenCAPI mode
		else {

			// Testing only if IBMVendorID => it needs to be improved to really test if CAPI or OpenCAPI card
			// (IBMvendorID has already been tested above => we cannot be here if vendorID != IBMvendorID)
			if strings.EqualFold(vendorID, IBMVendorID) {
				userDBDF := pciID
				healthy := pluginapi.Healthy // DBG: may need more investigation

				// get dsa version = fill it with image_loaded = Factory
				//fname = path.Join(SysfsDevices, pciID, ocxlFolder, ImageLoadFile)

				//content, err := GetFileContent(fname)
				content := ""

				// get Subsystem device id : capi2 + 0x668 = 9H7 card > fill it in dsaTs
				// get Subsystem device id : Ocapi + 0x666 = 9H7 card > fill it in dsaTs

				// get Subsystem device id (card_number) fill it in dsaTs
				fname = path.Join(SysfsDevices, pciID, SubDeviceFile)
				content, err = GetFileContent(fname)
				if err != nil {
					return nil, err
				}
				dsaTs := content

				// get device id
				fname = path.Join(SysfsDevices, pciID, DeviceFile)
				content, err = GetFileContent(fname)
				if err != nil {
					return nil, err
				}
				devid := content

				// OCP-CAPI-changes
				// Get CAPI card ID  (0 for card0, 1 for card1, etc) and then build CAPI device full path such as /dev/cxl/afu1.0m
				SysBusCXLPath := path.Join(SysfsDevices, pciID, "cxl")
				var CXLDevFullPath string
				if _, err := os.Stat(SysBusCXLPath); !os.IsNotExist(err) { // SysBusCXLPath (/sys/bus/pci/devices/<pciID>/cxl) exists  ==> CAPI Card
					card_name, _ := GetFileNameFromPrefix(SysBusCXLPath, CXLCardSTR)
					capiIDSTR := strings.TrimPrefix(card_name, CXLCardSTR)
					CXLDevFullPath = path.Join(CXLDevDir, CXLPrefix1+capiIDSTR+CXLPrefix2)
				} else { // SysBusCXLPath (/sys/bus/pci/devices/<pciID>/cxl) does NOT exist  ==> NO-CAPI Card
					CXLDevFullPath = ""
				}

				if strings.EqualFold(devid, CAPI2_V_ID) && strings.EqualFold(dsaTs, CAPI2_9H7) {
					content := "ad9h7_capi2"
					dsaVer := content
					devices = append(devices, Device{
						index:         strconv.Itoa(len(devices) + 1),
						shellVer:      dsaVer,
						timestamp:     dsaTs,
						DBDF:          userDBDF,
						deviceID:      devid,
						Healthy:       healthy,
						Nodes:         pairMap[DBD],
						CXLDevAFUPath: CXLDevFullPath,
					})
				} else if strings.EqualFold(devid, CAPI2_V_ID) && strings.EqualFold(dsaTs, CAPI2_U200) {
					content := "u200_capi2"
					dsaVer := content
					devices = append(devices, Device{
						index:         strconv.Itoa(len(devices) + 1),
						shellVer:      dsaVer,
						timestamp:     dsaTs,
						DBDF:          userDBDF,
						deviceID:      devid,
						Healthy:       healthy,
						Nodes:         pairMap[DBD],
						CXLDevAFUPath: CXLDevFullPath,
					})
				} else if strings.EqualFold(devid, OpencapiID) && strings.EqualFold(dsaTs, OCAPI_9H7) {
					content := "ad9h7_ocapi"
					dsaVer := content
					// /sys/bus/pci/devices/0004:00:00.1/ocxl*/ocxl exists only for opencapi virtual slot
					//  so if this directory doesn't exist => register the phys slot as not healthy
					_, err := os.Stat(path.Join(SysfsDevices, pciID, OcxlPrefix1+pciID, OcxlPrefix2))
					if os.IsNotExist(err) {
						// this logs the healthy physical slot - if IsExist used then log unhealthy virtual slot!!
						devices = append(devices, Device{
							index:         strconv.Itoa(len(devices) + 1),
							shellVer:      dsaVer,
							timestamp:     dsaTs,
							DBDF:          userDBDF,
							deviceID:      devid,
							Healthy:       healthy,
							Nodes:         pairMap[DBD],
							CXLDevAFUPath: "",
						})
					}
				}
				// end of OCP-CAPI-changes
			}
		}
	}

	// The function returns an array of Device structures ( []Device )
	return devices, nil
}

//#################################################################################################################################
/*
func main() {
	devices, err := GetDevices()
	if err != nil {
		fmt.Printf("%s !!!\n", err)
		return
	}
	for _, device := range devices {
		fmt.Printf("%v", device)
	}
}
*/
