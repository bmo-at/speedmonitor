package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/jackc/pgx/v4"
)

func InitializeDatabase() *pgx.Conn {
	database, err := pgx.Connect(context.Background(), "postgres://speedmonitor:k9xmtR4pQy8MXdCkfFng7S4iiTBiPhTFWGRzHSDE3o8UhvtzKyrBNPru4Gj8o6iowSGmhDXG3Ns4WMA98KA8HyicbVZ8CButM3NHQ8HPDNvABGrGqBcHbsEugcg3RTdE@dialga:5432/postgres")
	if err != nil {
		panic(err)
	}

	_, err = database.Exec(context.Background(), `
	CREATE TABLE IF NOT EXISTS speedmonitor(
		time TIMESTAMPTZ NOT NULL,
		ping REAL NOT NULL,
		jitter REAL NOT NULL,
		upload REAL NOT NULL,
		upload_time_ms REAL NOT NULL,
		upload_used_bytes REAL NOT NULL,
		download REAL NOT NULL,
		download_time_ms REAL NOT NULL,
		download_used_bytes REAL NOT NULL,
		isp TEXT NOT NULL,
		ip_external TEXT NOT NULL,
		traceroute TEXT NOT NULL,
		packet_loss REAL NOT NULL,
		url TEXT NOT NULL);`)
	if err != nil {
		panic(err)
	}

	return database
}

var database = InitializeDatabase()

var gdpr = struct {
	Settings struct {
		LicenseAccepted string `json:"LicenseAccepted"`
		GDPRTimeStamp   int64  `json:"GDPRTimeStamp"`
	} `json:"Settings"`
}{
	Settings: struct {
		LicenseAccepted string `json:"LicenseAccepted"`
		GDPRTimeStamp   int64  `json:"GDPRTimeStamp"`
	}{
		LicenseAccepted: "604ec27f828456331ebf441826292c49276bd3c1bee1a2f65a6452f505c4061c",
		GDPRTimeStamp:   time.Now().Unix(),
	},
}

type SpeedtestResult struct {
	Type       string
	Time       time.Time `json:"timestamp"`
	Latency    Latency   `json:"ping"`
	Download   UpDownload
	Upload     UpDownload
	PacketLoss float32
	ISP        string
	Interface  Interface
	Server     Server
	Result     Result
}

type UpDownload struct {
	Bandwidth int32
	Bytes     int32
	Elapsed   int32
}

type Interface struct {
	InternalIp string
	Name       string
	MacAddr    string
	IsVpn      bool
	ExternalIp string
}

type Server struct {
	Id       int32
	Host     string
	Port     int32
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
	Jitter float32
	Ping   float32 `json:"latency"`
}

func main() {
	defer database.Close(context.Background())

	sleepTime := 300

	if len(os.Args[1:]) > 0 {
		var err error = nil
		sleepTime, err = strconv.Atoi(os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
	}
	for {

		go testRoutine()
		log.Printf("Started speedtest, going to sleep for %d seconds", sleepTime)
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
}

func testRoutine() {
	cmd := exec.Command("speedtest", "-f", "json", "-s", "3692")

	var out bytes.Buffer

	cmd.Stdout = &out

	err := cmd.Run()

	if err != nil {
		log.Fatal(err)
	}

	var result SpeedtestResult

	err = json.Unmarshal(out.Bytes(), &result)

	if err != nil {
		log.Fatal(err)
	}

	cmd = exec.Command("traceroute", "google.com")

	var outTraceRoute bytes.Buffer

	cmd.Stdout = &outTraceRoute

	err = cmd.Run()

	if err != nil {
		log.Fatal(err)
	}

	traceRoute := outTraceRoute.String()

	_, err = database.Exec(context.Background(), `INSERT INTO speedmonitor(
		time,
		ping,
		jitter,
		upload,
		upload_time_ms,
		upload_used_bytes,
		download,
		download_time_ms,
		download_used_bytes,
		isp,
		ip_external,
		traceroute,
		packet_loss,
		url) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);`,
		result.Time,
		result.Latency.Ping,
		result.Latency.Jitter,
		result.Upload.Bandwidth*8/1_000_000,
		result.Upload.Elapsed,
		result.Upload.Bytes,
		result.Download.Bandwidth*8/1_000_000,
		result.Download.Elapsed,
		result.Download.Bytes,
		result.ISP,
		result.Interface.ExternalIp,
		traceRoute,
		result.PacketLoss,
		result.Result.Url,
	)
	if err != nil {
		log.Fatal(err)
	}
}
