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
	SysfsDevices   = "/sys/bus/pci/devices"
	CXLDevDir      = "/dev/cxl"
	OCXLDevDir     = "/dev/ocxl"
	CXLDevPrefix   = "afu"
	CXLDevPostfix  = ".0m"
	OCXLPrefix     = "ocxlfn."
	OCXLDevPrefix  = "IBM,oc-snap."
	OCXLDevPostfix = ".0"
	CXLDirName     = "cxl"
	OCXLDirName    = "ocxl"
	CXLCardSTR     = "card"
	IBMVendorID    = "0x1014"
	MgmtPrefix     = "/dev/xclmgmt"
	UserPrefix     = "/dev/dri"
	QdmaPrefix     = "/dev/xfpga"
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
	ADVANTECH_ID   = "0x13fe"
	AWS_ID         = "0x1d0f"
	AristaVendorID = "0x3475"
	CAPI2_P_ID     = "0x0477"
	CAPI2_V_ID     = "0x0632"
	OpencapiID     = "0x062b"
)

// Map for CAPI/OpenCAPI subdevices
// it is needed to include CAPI or OpenCAPI ID into the keys as some cards have the same SubID whenever they are CAPI or OpenCAPI
var CardMap = map[string]string{
	"0x0477_0x0665": "u200_capi2",
	"0x0477_0x0669": "u50_capi2",
	"0x0477_0x060f": "ad9v3_capi2",
	"0x0477_0x0667": "ad9h3_capi2",
	"0x0477_0x0668": "ad9h7_capi2",
	"0x062b_0x060f": "ad9v3_ocapi",
	"0x062b_0x0667": "ad9h3_ocapi",
	"0x062b_0x0666": "ad9h7_ocapi",
	"0x062b_0x066a": "bw250soc_ocapi",
}

type Pairs struct {
	Mgmt string
	User string
	Qdma string
}

type Device struct {
	index            string
	shellVer         string
	timestamp        string
	DBDF             string // this is for user pf
	deviceID         string //devid of the user pf
	Healthy          string
	Nodes            *Pairs
	CXL_OCXL_DevPath string
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

		// Get the device ID (CAPI=0x477, OpenCAPI=0x062b) of the card
		fdevice := path.Join(SysfsDevices, pciID, DeviceFile)
		deviceID, err := GetFileContent(fdevice)
		if err != nil {
			return nil, err
		}

		// If card vendorID is not IBMVendorID, then do not get device (do nothing) for this device
		// if card vendorID is IBMVendorID, but deviceID is not CAPI2_P_ID and is not OpencapiID, then do not get device (do nothing) for this device
		if strings.EqualFold(vendorID, IBMVendorID) != true ||
			(strings.EqualFold(deviceID, CAPI2_P_ID) != true && strings.EqualFold(deviceID, OpencapiID) != true) {
			continue
		}

		// If we get here, it means we found a CAPI or OpenCAPI card

		//for debugging only as it will pollute the logs
		//fmt.Println("Found CAPI/OpenCAPI card:", pciID, " (vendor ID=", vendorID, ", device ID=", deviceID, ")")
		//end debug

		// if pciID = "0003:01:00.0", DBD = "0003:01:00" (removing the last 2 characters)
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
				index:            strconv.Itoa(len(devices) + 1),
				shellVer:         dsaVer,
				timestamp:        dsaTs,
				DBDF:             userDBDF,
				deviceID:         devid,
				Healthy:          healthy,
				Nodes:            pairMap[DBD],
				CXL_OCXL_DevPath: "",
			})

			//-------------------------------------------------------------------------------------------------------------------------
			// mgmt_pf never occurs if CAPI/OpenCAPI Device
			// unless if XRT (Xilinx Runtime) is installed (then CAPI2 device may have a mgmt_pf file)
		} else if IsMgmtPf(pciID) { //mgmt pf
			// get mgmt instance
			fname = path.Join(SysfsDevices, pciID, InstanceFile)
			content, err := GetFileContent(fname)
			if err != nil {
				return nil, err
			}
			pairMap[DBD].Mgmt = MgmtPrefix + content

			//-------------------------------------------------------------------------------------------------------------------------
			// CAPI2 mode virtual slot or OpenCAPI mode
		} else {
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

			// CAPI2 case
			if strings.EqualFold(devid, CAPI2_P_ID) {

				// Get CAPI card ID  (0 for card0, 1 for card1, etc) and then build CAPI device full path such as /dev/cxl/afu1.0m
				SysBusCXLPath := path.Join(SysfsDevices, pciID, CXLDirName)
				var CXLDevFullPath string
				if _, err := os.Stat(SysBusCXLPath); !os.IsNotExist(err) { // SysBusCXLPath (/sys/bus/pci/devices/<pciID>/cxl) exists  ==> CAPI Card
					card_name, _ := GetFileNameFromPrefix(SysBusCXLPath, CXLCardSTR)
					capiIDSTR := strings.TrimPrefix(card_name, CXLCardSTR)
					CXLDevFullPath = path.Join(CXLDevDir, CXLDevPrefix+capiIDSTR+CXLDevPostfix)
					//} else { // SysBusCXLPath (/sys/bus/pci/devices/<pciID>/cxl) does NOT exist  ==> NO-CAPI Card
					//	CXLDevFullPath = ""
				}

				fID := devid + "_" + dsaTs
				content := CardMap[fID]
				dsaVer := content

				//for debugging only as it will pollute the logs
				// It could be a good idea to make the function loggin only once per device ID or use log.Debugf
				//fmt.Println("Registering CAPI2 card:", pciID, " (Device ID=", devid, ", SubDevice ID=", dsaTs, "Device Dir=", CXLDevFullPath, ")")
				//end debug

				devices = append(devices, Device{
					index:            strconv.Itoa(len(devices) + 1),
					shellVer:         dsaVer,
					timestamp:        dsaTs,
					DBDF:             userDBDF,
					deviceID:         devid,
					Healthy:          healthy,
					Nodes:            pairMap[DBD],
					CXL_OCXL_DevPath: CXLDevFullPath,
				})

				// OpenCAPI case
			} else if strings.EqualFold(devid, OpencapiID) {

				fID := devid + "_" + dsaTs
				content := CardMap[fID]
				dsaVer := content

				// /sys/bus/pci/devices/0004:00:00.1/ocxl*/ocxl exists only for opencapi virtual slot
				//  so only registering if this directory exists

				_, err := os.Stat(path.Join(SysfsDevices, pciID, OCXLPrefix+pciID, OCXLDirName))
				// If directory exists, Stat does NOT return any error so:
				//   -> IsNotExist(err) is false as err is NOT an error and so does NOT report that the directory exists
				// If directory does not exist, Stat returns an error reporting that the directory does not exists so:
				//   -> IsNotExist(err) is true as err is an error reporting that the directory does not exist

				// Testing below the opposite of os.IsNotExist in order to register only the device for which the directory exists
				// (os.IsExist would not be an option as it would be false in any case when analysing an error coming from os.Stat function)
				if !os.IsNotExist(err) {

					// Build OpenCAPI device full path such as /dev/ocxl/IBM,oc-snap.0004:00:00.1.0
					var OCXLDevFullPath string
					OCXLDevFullPath = path.Join(OCXLDevDir, OCXLDevPrefix+pciID+OCXLDevPostfix)

					//for debugging only as it will pollute the logs
					// It could be a good idea to make the function loggin only once per device ID or use log.Debugf
					//fmt.Println("Registering OpenCAPI card:", pciID, " (Device ID=", devid, ", SubDevice ID=", dsaTs, "Device Dir=", OCXLDevFullPath, ")")
					//end debug

					devices = append(devices, Device{
						index:            strconv.Itoa(len(devices) + 1),
						shellVer:         dsaVer,
						timestamp:        dsaTs,
						DBDF:             userDBDF,
						deviceID:         devid,
						Healthy:          healthy,
						Nodes:            pairMap[DBD],
						CXL_OCXL_DevPath: OCXLDevFullPath,
					})
				}
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
