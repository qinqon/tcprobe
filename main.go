package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/guptarohit/asciigraph"
)

const (
	clientMsg        = "ping"
	serverMsg        = "pong"
	curRow           = 2
	upperLimitRow    = 1
	buttonLimitRow   = 0
	rows             = 3
	interval         = time.Second / 8
	upperLimit       = 300 * time.Millisecond
	buttonLimit      = 130 * time.Millisecond
	maxLineLenBytes  = 1024
	readWriteTimeout = time.Minute
)

// ref madflojo.medium.com/keeping-tcp-connections-alive-in-golang-801a78b7cf1
func server(addr *net.TCPAddr) error {

	// Start TCP Listener
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return fmt.Errorf("Unable to start listener: %v", err)
	}

	for {
		// Wait for new connections and send them to reader()
		c, err := l.AcceptTCP()
		if err != nil {
			return fmt.Errorf("Listener returned: %v", err)
		}

		err = c.SetKeepAlive(false)
		if err != nil {
			return fmt.Errorf("Unable to set keepalive: %v", err)
		}
		go serveConnection(c)
	}
	return nil
}

func serveConnection(conn net.Conn) {
	log.Printf("accepted connection from %s", conn.RemoteAddr())

	defer func() {
		_ = conn.Close()
		log.Printf("closed connection from %s", conn.RemoteAddr())
	}()
	done := make(chan struct{})

	// time out one minute from now if no
	// data is received
	_ = conn.SetReadDeadline(time.Now().Add(readWriteTimeout))

	go func() {
		// limit the maximum line length (in bytes)
		lim := &io.LimitedReader{
			R: conn,
			N: maxLineLenBytes,
		}
		scan := bufio.NewScanner(lim)
		for scan.Scan() {
			input := scan.Text()
			output := ""
			if input == clientMsg {
				output = serverMsg
			}
			if _, err := conn.Write([]byte(output + "\n")); err != nil {
				log.Printf("failed to write output: %v", err)
				return
			}
			log.Printf("wrote response: %s", output)
			// reset the number of bytes remaining in the LimitReader
			lim.N = maxLineLenBytes
			// reset the read deadline
			_ = conn.SetReadDeadline(time.Now().Add(readWriteTimeout))
		}

		done <- struct{}{}
	}()

	<-done
}

func client(addr *net.TCPAddr) error {
	graph := NewResponseTimeGraph()
	// Open TCP Connection
	c, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return fmt.Errorf("Unable to dial to server: %v", err)
	}

	err = c.SetKeepAlive(false)
	if err != nil {
		return fmt.Errorf("Unable to set keepalive: %v", err)
	}
	for true {
		time.Sleep(interval)
		start := time.Now()
		_, err = fmt.Fprintf(c, clientMsg+"\n")
		if err != nil {
			return fmt.Errorf("Unable to send msg: %v", err)
		}
		msg, err := bufio.NewReader(c).ReadString('\n')
		if err != nil {
			return fmt.Errorf("Unable to read from server: %v", err)
		}
		elapsed := time.Since(start)
		msg = strings.TrimSuffix(msg, "\n")
		if msg != serverMsg {
			return fmt.Errorf("Received unexpected server message: %s", msg)
		}
		graph.Plot(elapsed)
	}
	return nil
}

func durationToSample(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

type ResponseTimeGraph struct {
	data                          [][]float64
	buffer, height, width, offset int
	precision                     uint
	max                           time.Duration
}

func NewResponseTimeGraph() ResponseTimeGraph {
	return ResponseTimeGraph{
		buffer:    130,
		height:    23,
		offset:    5,
		precision: 3,
		data:      [][]float64{{}, {}, {}},
	}
}

func (g *ResponseTimeGraph) Plot(elapsed time.Duration) {
	if elapsed > g.max {
		g.max = elapsed
	}
	if len(g.data[curRow]) > g.buffer {
		for _, row := range []int{curRow, upperLimitRow, buttonLimitRow} {
			g.data[row] = g.data[row][len(g.data[row])-g.buffer:]
		}

	}
	sample := durationToSample(elapsed)
	max := durationToSample(g.max)
	sampleToPlot := sample
	if elapsed > upperLimit {
		sampleToPlot = durationToSample(upperLimit + time.Millisecond)
	}
	g.data[curRow] = append(g.data[curRow], sampleToPlot)
	g.data[upperLimitRow] = append(g.data[upperLimitRow], durationToSample(upperLimit))
	g.data[buttonLimitRow] = append(g.data[buttonLimitRow], durationToSample(buttonLimit))
	graph := asciigraph.PlotMany(g.data,
		asciigraph.Height(g.height),
		asciigraph.Offset(g.offset),
		asciigraph.Precision(g.precision),
		asciigraph.Caption(fmt.Sprintf("Response time [%.3f ms], max [%.3f ms]", sample, max)),
		asciigraph.SeriesColors(
			asciigraph.Black,
			asciigraph.Black,
			asciigraph.Yellow,
		),
	)

	asciigraph.Clear()
	fmt.Println(graph)
}

func main() {
	kind := os.Args[1]
	addr := os.Args[2]

	// Resolve TCP Address
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic("Unable to resolve IP")
	}
	if kind == "s" {
		err = server(tcpAddr)

	} else if kind == "c" {
		err = client(tcpAddr)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
