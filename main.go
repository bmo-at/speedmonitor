package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"
	speedtest_go "github.com/showwin/speedtest-go/speedtest"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PingEntry struct {
	Time         time.Time `gorm:"not null"`
	Rtt_min      float64   `gorm:"not null"`
	Rtt_max      float64   `gorm:"not null"`
	Rtt_avg      float64   `gorm:"not null"`
	Rtt_mdev     float64   `gorm:"not null"`
	Packet_loss  float64   `gorm:"not null"`
	Endpoint_url string    `gorm:"not null"`
}

func (PingEntry) TableName() string {
	return "pingmonitor"
}

type SpeedtestEntry struct {
	Time                time.Time `gorm:"not null"`
	Ping                float64   `gorm:"not null"`
	Jitter              float64   `gorm:"not null"`
	Upload              float64   `gorm:"not null"`
	Download            float64   `gorm:"not null"`
	Packet_loss         float64   `gorm:"not null"`
	Url                 string    `gorm:"not null"`
	Upload_time_ms      float64   `gorm:"not null;default:0"`
	Download_time_ms    float64   `gorm:"not null;default:0"`
	Upload_used_bytes   float64   `gorm:"not null;default:0"`
	Download_used_bytes float64   `gorm:"not null;default:0"`
	Isp                 string    `gorm:"not null;default:'default'"`
	Ip_external         string    `gorm:"not null;default:'127.0.0.1'"`
	Traceroute          string    `gorm:"not null;default:'traceroute'"`
}

func (SpeedtestEntry) TableName() string {
	return "speedmonitor"
}

type PingResult struct {
	DestinationIP      string      `json:"destination_ip"`
	DataBytes          int         `json:"data_bytes"`
	Pattern            interface{} `json:"pattern"`
	Destination        string      `json:"destination"`
	PacketsTransmitted int         `json:"packets_transmitted"`
	PacketsReceived    int         `json:"packets_received"`
	PacketLossPercent  float64     `json:"packet_loss_percent"`
	Duplicates         int         `json:"duplicates"`
	TimeMs             float64     `json:"time_ms"`
	RoundTripMsMin     float64     `json:"round_trip_ms_min"`
	RoundTripMsAvg     float64     `json:"round_trip_ms_avg"`
	RoundTripMsMax     float64     `json:"round_trip_ms_max"`
	RoundTripMsStddev  float64     `json:"round_trip_ms_stddev"`
	Responses          []struct {
		Type       string    `json:"type"`
		Timestamp  time.Time `json:"timestamp"`
		Bytes      int       `json:"bytes"`
		ResponseIP string    `json:"response_ip"`
		IcmpSeq    int       `json:"icmp_seq"`
		TTL        int       `json:"ttl"`
		TimeMs     float64   `json:"time_ms"`
		Duplicate  bool      `json:"duplicate"`
	} `json:"responses"`
}

type SpeedtestResult struct {
	Type       string
	Time       time.Time `json:"timestamp"`
	Latency    Latency   `json:"ping"`
	Download   UpDownload
	Upload     UpDownload
	PacketLoss float64
	ISP        string
	Interface  Interface
	Server     Server
	Result     Result
}

type UpDownload struct {
	Bandwidth int64
	Bytes     int64
	Elapsed   int64
}

type Interface struct {
	InternalIp string
	Name       string
	MacAddr    string
	IsVpn      bool
	ExternalIp string
}

type Server struct {
	Id       int64
	Host     string
	Port     int64
	Name     string
	Location string
	Country  string
	Ip       string
}

type Result struct {
	Id        string
	Url       string
	Persisted bool
}

type Latency struct {
	Jitter float64
	Ping   float64 `json:"latency"`
}

var wg sync.WaitGroup

func main() {

	db_user := "postgres"

	if value, exists := os.LookupEnv("INFRAMONITOR_DB_USER"); exists {
		db_user = value
	} else {
		log.Printf("Environment variable %s not set, using default value %s", "INFRAMONITOR_DB_USER", db_user)
	}

	db_password := "inframonitor_dev_password"

	if value, exists := os.LookupEnv("INFRAMONITOR_DB_PASSWORD"); exists {
		db_password = value
	} else {
		log.Printf("Environment variable %s not set, using default value %s", "INFRAMONITOR_DB_PASSWORD", db_password)
	}

	db_host := "localhost"

	if value, exists := os.LookupEnv("INFRAMONITOR_DB_HOST"); exists {
		db_host = value
	} else {
		log.Printf("Environment variable %s not set, using default value %s", "INFRAMONITOR_DB_HOST", db_host)
	}

	db_port := "5432"

	if value, exists := os.LookupEnv("INFRAMONITOR_DB_PORT"); exists {
		db_port = value
	} else {
		log.Printf("Environment variable %s not set, using default value %s", "INFRAMONITOR_DB_PORT", db_port)
	}

	db, err := gorm.Open(postgres.Open(fmt.Sprintf("host=%s user=%s password=%s dbname=postgres port=%s", db_host, db_user, db_password, db_port)), &gorm.Config{})

	if err != nil {
		log.Fatalf("failed to connect to database: %v", err.Error())
	}

	db.AutoMigrate(&PingEntry{}, SpeedtestEntry{})

	var tables []struct {
		Table_name string
	}

	db.Raw("SELECT table_name FROM _timescaledb_catalog.hypertable").Scan(&tables)

	hypertable_pingmonitor_exists := false
	hypertable_speedmonitor_exists := false

	for _, table := range tables {
		if strings.Compare(table.Table_name, SpeedtestEntry.TableName(SpeedtestEntry{})) == 0 {
			hypertable_speedmonitor_exists = true
		}
		if strings.Compare(table.Table_name, PingEntry.TableName(PingEntry{})) == 0 {
			hypertable_pingmonitor_exists = true
		}
	}

	if !hypertable_pingmonitor_exists {
		log.Printf("Hypertable %s does not yet exist, creating...", PingEntry.TableName(PingEntry{}))
		db.Exec("SELECT create_hypertable(?, 'time')", PingEntry.TableName(PingEntry{}))
	} else {
		log.Printf("Hypertable %s already exists, skipping creation", PingEntry.TableName(PingEntry{}))
	}
	if !hypertable_speedmonitor_exists {
		log.Printf("Hypertable %s does not yet exist, creating...", SpeedtestEntry.TableName(SpeedtestEntry{}))
		db.Exec("SELECT create_hypertable(?, 'time')", SpeedtestEntry.TableName(SpeedtestEntry{}))
	} else {
		log.Printf("Hypertable %s already exists, skipping creation", SpeedtestEntry.TableName(SpeedtestEntry{}))
	}

	sleepTimePing := 1
	sleepTimeSpeedtest := 300

	if value, exists := os.LookupEnv("INFRAMONITOR_SLEEP_TIME_SPEEDTEST"); exists {
		sleepTimeSpeedtest, err = strconv.Atoi(value)
		if err != nil {
			log.Fatalf("Error while parsing argument %s", err.Error())
		}
	} else {
		log.Printf("Environment variable %s not set, using default value %d", "INFRAMONITOR_SLEEP_TIME_SPEEDTEST", sleepTimePing)
	}

	if value, exists := os.LookupEnv("INFRAMONITOR_SLEEP_TIME_PING"); exists {
		sleepTimePing, err = strconv.Atoi(value)
		if err != nil {
			log.Fatalf("Error while parsing argument %s", err.Error())
		}
	} else {
		log.Printf("Environment variable %s not set, using default value %d", "INFRAMONITOR_SLEEP_TIME_PING", sleepTimeSpeedtest)
	}

	serverID := ""

	if value, exists := os.LookupEnv("INFRAMONITOR_SPEEDTEST_SERVER_ID"); exists {
		serverID = value
	} else {
		log.Printf("Environment variable %s not set, using closest server instead", "INFRAMONITOR_SPEEDTEST_SERVER_ID")
	}

	error_channel := make(chan error)

	wg.Add(2)

	go speedtestRoutine(db, error_channel, sleepTimeSpeedtest, serverID)
	go pingRoutine(db, error_channel, sleepTimePing)

	wg.Wait()
}

func pingRoutine(db *gorm.DB, error_channel chan error, sleepTime int) {
	log.Println("Starting ping subroutines")

	destinations := []string{"google.com", "1.1.1.1"}

	count := 1

	if value, exists := os.LookupEnv("INFRAMONITOR_DESTINATIONS_PING"); exists {
		destinations = strings.Split(value, ",")
	} else {
		log.Printf("Environment variable %s not set, using default value %v", "INFRAMONITOR_DESTINATIONS_PING", destinations)
	}

	if value, exists := os.LookupEnv("INFRAMONITOR_COUNT_PING"); exists {
		c, err := strconv.Atoi(value)
		if err != nil {
			log.Fatalf("Error while parsing argument %s", err.Error())
		}
		count = c
	} else {
		log.Printf("Environment variable %s not set, using default value %d", "INFRAMONITOR_COUNT_PING", count)
	}

	for {
		for _, d := range destinations {
			log.Printf("Starting ping subroutine for target '%s'", d)
			go ping(d, count, db)
		}
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
}

func ping(destination_url string, count int, db *gorm.DB) {
	log.Printf("Starting ping command for target '%s'", destination_url)

	pinger, err := probing.NewPinger(destination_url)

	if err != nil {
		log.Printf("Error creating pinger for target '%s': %s", destination_url, err.Error())
	}

	pinger.Count = count
	pinger.Timeout = 3 * time.Second

	err = pinger.Run()

	if err != nil {
		log.Printf("Error while pinging target '%s': %s", destination_url, err.Error())
	}

	log.Printf("Ping command for target '%s' finished", destination_url)

	result := pinger.Statistics()

	err = db.Create(&PingEntry{
		Time:         time.Now(),
		Rtt_min:      float64(result.MinRtt.Milliseconds()),
		Rtt_max:      float64(result.MaxRtt.Milliseconds()),
		Rtt_avg:      float64(result.AvgRtt.Milliseconds()),
		Rtt_mdev:     float64(result.StdDevRtt.Milliseconds()),
		Packet_loss:  result.PacketLoss,
		Endpoint_url: destination_url,
	}).Error

	if err != nil {
		log.Fatalf("Error while inserting into DB: %s", err.Error())
	}

	log.Printf("Ping for target '%s' complete, result saved", destination_url)
}

func speedtestRoutine(db *gorm.DB, error_channel chan error, sleepTime int, serverID string) {
	var server *speedtest_go.Server

	speedtestClient := speedtest_go.New()

	if len(serverID) > 0 {
		server, _ = speedtestClient.FetchServerByID(serverID)
	} else {
		serverList, _ := speedtest_go.FetchServers()
		temp, _ := serverList.FindServer([]int{})
		server = temp[0]
	}

	if server == nil {
		panic("Server can not be nil!")
	}

	for {
		log.Println("Starting speedtest subroutine")
		go speedtest(db, server)
		log.Printf("Sleeping after speedtest routine for %d seconds", sleepTime)
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
}

func speedtest(db *gorm.DB, server *speedtest_go.Server) {
	log.Println("Starting speedtest command")

	err := server.TestAll()

	if err != nil {
		log.Fatalf("Error while running the speedtest command: %s", err.Error())
	}

	log.Println("Speedtest command finished")

	// log.Println("Starting traceroute command")

	// cmd := exec.Command("traceroute", "google.com")

	// var outTraceRoute bytes.Buffer

	// cmd.Stdout = &outTraceRoute

	// err = cmd.Run()

	// if err != nil {
	// 	log.Fatalf("Error while running the traceroute command: %s", err.Error())
	// }

	// log.Println("Traceroute command finished")

	// traceRoute := outTraceRoute.String()

	err = db.Create(&SpeedtestEntry{
		Time:                time.Now(),
		Ping:                float64(server.Latency.Milliseconds()) / 1000,
		Jitter:              float64(server.Jitter.Milliseconds()) / 1000,
		Upload:              float64(server.ULSpeed.Mbps()),
		Download:            float64(server.DLSpeed.Mbps()),
		Packet_loss:         server.PacketLoss.Loss(),
		Url:                 server.URL,
		Upload_time_ms:      float64(server.TestDuration.Download.Milliseconds()) / 1000,
		Download_time_ms:    float64(server.TestDuration.Upload.Milliseconds()) / 1000,
		Upload_used_bytes:   float64(0),
		Download_used_bytes: float64(0),
		Isp:                 server.Host,
		Ip_external:         server.Country,
		Traceroute:          "",
	}).Error

	if err != nil {
		log.Fatalf("Error while inserting into DB: %s", err.Error())
	}

	log.Println("Speedtest complete, result saved")
}
