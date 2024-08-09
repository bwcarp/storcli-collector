package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

const Namespace = "megaraid"
const Version = "0.1.0"

var StorcliPath string

type PhysicalDrive struct {
	EIDSlt string `json:"EID:Slt"`
	DID    int    `json:"DID"`
	Intf   string `json:"Intf"`
	Med    string `json:"Med"`
	Model  string `json:"Model"`
	DG     int    `json:"DG"`
	State  string `json:"State"`
}

type PhysicalDriveUnpack struct {
	Controllers []struct {
		ResponseData map[string]interface{} `json:"Response Data"`
	} `json:"Controllers"`
}

type Controller struct {
	CommandStatus struct {
		Status string `json:"Status"`
	} `json:"Command Status"`
	ResponseData struct {
		Basics struct {
			Controller     int    `json:"Controller"`
			Model          string `json:"Model"`
			SerialNumber   string `json:"Serial Number"`
			ControllerDate string `json:"Current Controller Date/Time"`
			SystemDate     string `json:"Current System Date/time"`
		} `json:"Basics"`
		Version struct {
			DriverName      string `json:"Driver Name"`
			FirmwareVersion string `json:"Firmware Version"`
		} `json:"Version"`
		Status struct {
			ControllerStatus string `json:"Controller Status"`
			BBUStatus        int    `json:"BBU Status"`
		} `json:"Status"`
		HwCfg struct {
			BackendPortCount int `json:"Backend Port Count"`
			// spelling can vary
			ROCTempCelsius int `json:"ROC temperature(Degree Celsius)"`
			ROCTempCelcius int `json:"ROC temperature(Degree Celcius)"`
		} `json:"HwCfg"`
		ScheduledTasks struct {
			PatrolReadReoccurrence string `json:"Patrol Read Reoccurrence"`
		} `json:"Scheduled Tasks"`
		DriveGroups   int `json:"Drive Groups"`
		VirtualDrives int `json:"Virtual Drives"`
		VDList        []struct {
			DG_VD string `json:"DG/VD"`
			Name  string `json:"Name"`
			Cache string `json:"Cache"`
			Type  string `json:"TYPE"`
			State string `json:"State"`
		} `json:"VD LIST"`
		PhysicalDrives int             `json:"Physical Drives"`
		PDList         []PhysicalDrive `json:"PD LIST"`
		CachevaultInfo []struct {
			Temp string `json:"Temp"`
		} `json:"Cachevault_Info"`
		BBUInfo []struct {
			Temp string `json:"Temp"`
		} `json:"BBU_Info"`
	} `json:"Response Data"`
}

type ControllerData struct {
	Controllers []Controller `json:"Controllers"`
}

var Metrics = map[string]*prometheus.GaugeVec{
	"ctrl_info": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "controller_info",
			Help:      "MegaRAID controller info",
		},
		[]string{"controller", "model", "serial", "fwversion"},
	),
	"ctrl_temperature": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "temperature",
			Help:      "MegaRAID controller temperature",
		},
		[]string{"controller"},
	),
	"ctrl_healthy": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "healthy",
			Help:      "MegaRAID controller healthy",
		},
		[]string{"controller"},
	),
	"ctrl_degraded": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "degraded",
			Help:      "MegaRAID controller degraded",
		},
		[]string{"controller"},
	),
	"ctrl_failed": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "failed",
			Help:      "MegaRAID controller failed",
		},
		[]string{"controller"},
	),
	"ctrl_time_difference": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "time_difference",
			Help:      "MegaRAID controller failed",
		},
		[]string{"controller"},
	),
	"bbu_healthy": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "battery_backup_healthy",
			Help:      "MegaRAID battery backup healthy",
		},
		[]string{"controller"},
	),
	"bbu_temperature": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "bbu_temperature",
			Help:      "MegaRAID battery backup temperature",
		},
		[]string{"controller", "bbuidx"},
	),
	"cv_temperature": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "cv_temperature",
			Help:      "MegaRAID CacheVault temperature",
		},
		[]string{"controller", "cvidx"},
	),
	"ctrl_sched_patrol_read": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "scheduled_patrol_read",
			Help:      "MegaRAID scheduled patrol read",
		},
		[]string{"controller"},
	),
	"ctrl_ports": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "ports",
			Help:      "MegaRAID ports",
		},
		[]string{"controller"},
	),
	"ctrl_physical_drives": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "physical_drives",
			Help:      "MegaRAID physical drives",
		},
		[]string{"controller"},
	),
	"ctrl_drive_groups": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "drive_groups",
			Help:      "MegaRAID drive groups",
		},
		[]string{"controller"},
	),
	"ctrl_virtual_drives": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "virtual_drives",
			Help:      "MegaRAID virtual drives",
		},
		[]string{"controller"},
	),
	"vd_info": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "vd_info",
			Help:      "MegaRAID virtual drive info",
		},
		[]string{"controller", "DG", "VG", "name", "cache", "type", "state"},
	),
	"pd_shield_counter": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_shield_counter",
			Help:      "MegaRAID physical drive shield counter",
		},
		[]string{"controller", "enclosure", "slot"},
	),
	"pd_media_errors": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_media_errors",
			Help:      "MegaRAID physical drive media errors",
		},
		[]string{"controller", "enclosure", "slot"},
	),
	"pd_other_errors": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_other_errors",
			Help:      "MegaRAID physical drive other errors",
		},
		[]string{"controller", "enclosure", "slot"},
	),
	"pd_predictive_errors": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_predictive_errors",
			Help:      "MegaRAID physical drive predictive errors",
		},
		[]string{"controller", "enclosure", "slot"},
	),
	"pd_smart_alerted": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_smart_alerted",
			Help:      "MegaRAID physical drive SMART alerted",
		},
		[]string{"controller", "enclosure", "slot"},
	),
	"pd_link_speed": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_link_speed_gbps",
			Help:      "MegaRAID physical drive link speed in Gbps",
		},
		[]string{"controller", "enclosure", "slot"},
	),
	"pd_device_speed": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_device_speed_gbps",
			Help:      "MegaRAID physical drive device speed in Gbps",
		},
		[]string{"controller", "enclosure", "slot"},
	),
	"pd_commissioned_spare": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_commissioned_spare",
			Help:      "MegaRAID physical drive commissioned spare",
		},
		[]string{"controller", "enclosure", "slot"},
	),
	"pd_emergency_spare": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_emergency_spare",
			Help:      "MegaRAID physical drive emergency spare",
		},
		[]string{"controller", "enclosure", "slot"},
	),
	"pd_info": prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pd_info",
			Help:      "MegaRAID physical drive info",
		},
		[]string{
			"controller",
			"enclosure",
			"slot",
			"disk_id",
			"interface",
			"media",
			"model",
			"DG",
			"state",
			"firmware",
			"serial",
		},
	),
}

func getStorcliJson() ControllerData {

	if _, err := os.Stat(StorcliPath); os.IsNotExist(err) {
		log.Fatal(err)
	}

	data, err := exec.Command(StorcliPath, "/cALL", "show", "all", "J").Output()
	if err != nil {
		log.Fatal(err)
	}

	/* TEST CASE - Temporarily use a text file
	data, err := os.ReadFile("controllers.json")
	if err != nil {
		log.Fatal(err)
	}
	*/

	// Because this thing will return a string of NA if the
	// BBU doesn't exist, which won't unpack into the struct.
	// Why though?
	dataString := string(data)
	dataString = strings.Replace(dataString, `"BBU Status" : "NA"`, `"BBU Status" : 9999`, 1)
	dataString = strings.Replace(dataString, `"DG" : "-"`, `"DG" : 9999`, -1)
	data = []byte(dataString)

	var getControllers ControllerData
	err = json.Unmarshal(data, &getControllers)
	if err != nil {
		log.Fatal(err)
	}

	if getControllers.Controllers[0].CommandStatus.Status != "Success" {
		log.Fatal("Could not find controllers in output.")
	}

	return getControllers
}

func getStorcliDrivesJson() PhysicalDriveUnpack {

	/* TEST CASE - Temporarily use a text file
	data, err := os.ReadFile("drives.json")
	if err != nil {
		log.Fatal(err)
	}
	*/

	data, err := exec.Command(StorcliPath, "/cALL/eALL/sALL", "show", "all", "J").Output()
	if err != nil {
		log.Fatal(err)
	}

	var jsonOutput PhysicalDriveUnpack
	err = json.Unmarshal(data, &jsonOutput)
	if err != nil {
		log.Fatal(err)
	}

	return jsonOutput
}

func printMetrics(reg *prometheus.Registry) string {

	g := prometheus.Gatherers{reg}
	gatheredMetrics, err := g.Gather()
	if err != nil {
		log.Fatal(err)
	}

	buf := new(bytes.Buffer)
	for _, metric := range gatheredMetrics {
		_, err = expfmt.MetricFamilyToOpenMetrics(buf, metric)
		if err != nil {
			log.Fatal(err)
		}
	}

	return (buf.String())

}

func handleCommonController(controller Controller) {

	controllerIndex := strconv.Itoa(controller.ResponseData.Basics.Controller)

	Metrics["ctrl_info"].With(prometheus.Labels{
		"controller": controllerIndex,
		"model":      controller.ResponseData.Basics.Model,
		"serial":     controller.ResponseData.Basics.SerialNumber,
		"fwversion":  controller.ResponseData.Version.FirmwareVersion,
	}).Set(1)

	var tempCelsius float64
	if controller.ResponseData.HwCfg.ROCTempCelcius > 0 {
		tempCelsius = float64(controller.ResponseData.HwCfg.ROCTempCelcius)
	} else if controller.ResponseData.HwCfg.ROCTempCelsius > 0 {
		tempCelsius = float64(controller.ResponseData.HwCfg.ROCTempCelsius)
	} else {
		tempCelsius = 0
	}

	Metrics["ctrl_temperature"].With(prometheus.Labels{
		"controller": controllerIndex,
	}).Set(tempCelsius)

}

func handleMegaraidController(controller Controller) {

	controllerIndex := strconv.Itoa(controller.ResponseData.Basics.Controller)

	var bbuStatus float64
	switch controller.ResponseData.Status.BBUStatus {
	case 0:
		bbuStatus = 1
	case 8:
		bbuStatus = 1
	case 4096:
		bbuStatus = 1
	default:
		bbuStatus = 0
	}
	Metrics["bbu_healthy"].With(prometheus.Labels{
		"controller": controllerIndex,
	}).Set(bbuStatus)

	var controllerStatusDegraded float64
	var controllerStatusFailed float64
	var controllerStatusOptimal float64

	switch controller.ResponseData.Status.ControllerStatus {
	case "Degraded":
		controllerStatusDegraded = 1
	case "Failed":
		controllerStatusFailed = 1
	case "Optimal":
		controllerStatusOptimal = 1
	}

	Metrics["ctrl_degraded"].With(prometheus.Labels{
		"controller": controllerIndex,
	}).Set(controllerStatusDegraded)
	Metrics["ctrl_failed"].With(prometheus.Labels{
		"controller": controllerIndex,
	}).Set(controllerStatusFailed)
	Metrics["ctrl_healthy"].With(prometheus.Labels{
		"controller": controllerIndex,
	}).Set(controllerStatusOptimal)

	Metrics["ctrl_ports"].With(prometheus.Labels{
		"controller": controllerIndex,
	}).Set(float64(controller.ResponseData.HwCfg.BackendPortCount))

	var scheduledPatrolRead float64
	if strings.Contains(controller.ResponseData.ScheduledTasks.PatrolReadReoccurrence, "hrs") {
		scheduledPatrolRead = 1
	}
	Metrics["ctrl_sched_patrol_read"].With(prometheus.Labels{
		"controller": controllerIndex,
	}).Set(scheduledPatrolRead)

	for cvidx, cvinfo := range controller.ResponseData.CachevaultInfo {
		tempString := strings.Replace(cvinfo.Temp, "C", "", 1)
		temperature, _ := strconv.ParseFloat(tempString, 64)
		Metrics["cv_temperature"].With(prometheus.Labels{
			"controller": controllerIndex,
			"cvidx":      strconv.Itoa(cvidx),
		}).Set(temperature)
	}

	for bbuidx, bbuinfo := range controller.ResponseData.BBUInfo {
		tempString := strings.Replace(bbuinfo.Temp, "C", "", 1)
		temperature, _ := strconv.ParseFloat(tempString, 64)
		Metrics["bbu_temperature"].With(prometheus.Labels{
			"controller": controllerIndex,
			"bbuidx":     strconv.Itoa(bbuidx),
		}).Set(temperature)
	}

	timefmt := "01/02/2006, 15:04:05"

	if controller.ResponseData.Basics.ControllerDate != "" && controller.ResponseData.Basics.SystemDate != "" {
		controllerDateTime, conErr := time.Parse(timefmt, controller.ResponseData.Basics.ControllerDate)
		systemDateTime, sysErr := time.Parse(timefmt, controller.ResponseData.Basics.SystemDate)
		if conErr == nil || sysErr == nil {
			timeDiff := float64(systemDateTime.Unix() - controllerDateTime.Unix())
			Metrics["ctrl_time_difference"].With(prometheus.Labels{
				"controller": controllerIndex,
			}).Set(timeDiff)
		}
	}

	if controller.ResponseData.DriveGroups > 0 {
		Metrics["ctrl_drive_groups"].With(prometheus.Labels{
			"controller": controllerIndex,
		}).Set(float64(controller.ResponseData.DriveGroups))
		Metrics["ctrl_virtual_drives"].With(prometheus.Labels{
			"controller": controllerIndex,
		}).Set(float64(controller.ResponseData.VirtualDrives))

		for _, virtualDrive := range controller.ResponseData.VDList {
			var driveGroup string = "-1"
			var volumeGroup string = "-1"
			if virtualDrive.DG_VD != "" {
				groups := strings.Split(virtualDrive.DG_VD, "/")
				driveGroup = groups[0]
				volumeGroup = groups[1]
			}
			Metrics["vd_info"].With(prometheus.Labels{
				"controller": controllerIndex,
				"DG":         driveGroup,
				"VG":         volumeGroup,
				"name":       virtualDrive.Name,
				"cache":      virtualDrive.Cache,
				"type":       virtualDrive.Type,
				"state":      virtualDrive.State,
			}).Set(1)
		}
	}

	Metrics["ctrl_physical_drives"].With(prometheus.Labels{
		"controller": controllerIndex,
	}).Set(float64(controller.ResponseData.PhysicalDrives))

	if controller.ResponseData.PhysicalDrives > 0 {
		data := getStorcliDrivesJson()
		driveInfo := data.Controllers[controller.ResponseData.Basics.Controller].ResponseData
		for _, physicalDrive := range controller.ResponseData.PDList {
			createMetricsOfPhysicalDrive(physicalDrive, driveInfo, controllerIndex)
		}
	}
}

func createMetricsOfPhysicalDrive(physicalDrive PhysicalDrive, detailedInfoArray map[string]interface{}, controllerIndex string) {

	splitEIDSlt := strings.Split(physicalDrive.EIDSlt, ":")
	enclosure := splitEIDSlt[0]
	slot := splitEIDSlt[1]

	var driveIdentifier string
	if enclosure == " " {
		driveIdentifier = fmt.Sprintf("Drive /c%s/s%s", controllerIndex, slot)
		enclosure = ""
	} else {
		driveIdentifier = fmt.Sprintf("Drive /c%s/e%s/s%s", controllerIndex, enclosure, slot)
	}

	info := detailedInfoArray[driveIdentifier+" - Detailed Information"].(map[string]interface{})
	state := info[driveIdentifier+" State"].(map[string]interface{})
	attributes := info[driveIdentifier+" Device attributes"].(map[string]interface{})
	settings := info[driveIdentifier+" Policies/Settings"].(map[string]interface{})

	Metrics["pd_shield_counter"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
	}).Set(state["Shield Counter"].(float64))
	Metrics["pd_media_errors"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
	}).Set(state["Media Error Count"].(float64))
	Metrics["pd_other_errors"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
	}).Set(state["Other Error Count"].(float64))
	Metrics["pd_predictive_errors"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
	}).Set(state["Predictive Failure Count"].(float64))
	var smartAlerted float64
	if state["S.M.A.R.T alert flagged by drive"].(string) == "Yes" {
		smartAlerted = 1.0
	}
	Metrics["pd_smart_alerted"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
	}).Set(smartAlerted)

	linkSpeedAttr := strings.Split(attributes["Link Speed"].(string), ".")
	linkSpeed, _ := strconv.ParseFloat(linkSpeedAttr[0], 64)
	Metrics["pd_link_speed"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
	}).Set(linkSpeed)
	deviceSpeedAttr := strings.Split(attributes["Device Speed"].(string), ".")
	deviceSpeed, _ := strconv.ParseFloat(deviceSpeedAttr[0], 64)
	Metrics["pd_device_speed"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
	}).Set(deviceSpeed)

	var commissionedSpare float64
	var emergencySpare float64
	if settings["Commissioned Spare"].(string) == "Yes" {
		commissionedSpare = 1.0
	}
	if settings["Emergency Spare"].(string) == "Yes" {
		emergencySpare = 1.0
	}
	Metrics["pd_commissioned_spare"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
	}).Set(commissionedSpare)
	Metrics["pd_emergency_spare"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
	}).Set(emergencySpare)

	model := strings.Replace(physicalDrive.Model, " ", "", -1)
	firmware := strings.Replace(attributes["Firmware Revision"].(string), " ", "", -1)
	serial := strings.Replace(attributes["SN"].(string), " ", "", -1)

	// Because sometimes it's not part of a device group.
	dgFixed := "-"
	if physicalDrive.DG != 9999 {
		dgFixed = strconv.Itoa(physicalDrive.DG)
	}
	Metrics["pd_info"].With(prometheus.Labels{
		"controller": controllerIndex,
		"enclosure":  enclosure,
		"slot":       slot,
		"disk_id":    strconv.Itoa(physicalDrive.DID),
		"interface":  physicalDrive.Intf,
		"media":      physicalDrive.Med,
		"model":      model,
		"DG":         dgFixed,
		"state":      physicalDrive.State,
		"firmware":   firmware,
		"serial":     serial,
	}).Set(1)
}

func main() {

	var storcliPath = flag.String("storcli_path", "/opt/MegaRAID/storcli/storcli64", "(Optional) Absolute path to StorCLI binary. Defaults to /opt/MegaRAID/storcli/storcli64 or storcli in PATH")
	var storcliDontfail = flag.Bool("storcli_dontfailover", false, "(Optional) Don't fall back to PATH env if absolute path is missing.")
	var version = flag.Bool("version", false, "Get version information")
	var outputFile = flag.String("outfile", "", "Text file to write output to. Defaults to standard output.")

	flag.Parse()

	if *version {
		fmt.Println(Version)
		os.Exit(0)
	}

	// In testing I found that even if storcli is in the user's PATH,
	// exec.Command won't find it.
	if _, err := os.Stat(*storcliPath); err == nil {
		StorcliPath = *storcliPath
	} else if *storcliDontfail {
		log.Fatal(err)
	} else {
		folders := strings.Split(os.Getenv("PATH"), ":")
		for _, folder := range folders {
			executable := fmt.Sprintf("%s/storcli", folder)
			if _, err := os.Stat(executable); err == nil {
				StorcliPath = executable
				break
			}
		}
		if StorcliPath == "" {
			log.Fatal("storcli not found.")
		}
	}

	getControllers := getStorcliJson()

	reg := prometheus.NewRegistry()
	for _, v := range Metrics {
		reg.MustRegister(v)
	}

	for _, controller := range getControllers.Controllers {
		handleCommonController(controller)
		if controller.ResponseData.Version.DriverName == "megaraid_sas" {
			handleMegaraidController(controller)
		}
	}

	if *outputFile != "" {
		err := os.WriteFile(*outputFile, []byte(printMetrics(reg)), 0644)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Print(printMetrics(reg))
	}
}
