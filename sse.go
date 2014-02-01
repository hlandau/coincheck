package main
import "bufio"
import "strconv"
import "bytes"
import "errors"
import "fmt"
import "net"
import "strings"
import "io"
import "time"
import "log"

const sPreHeaders = 0
const sLength     = 1
const sData       = 2
const sPostChunk  = 3

const sTimeout    = 30*time.Second
const sRetryDelay = 10*time.Second

type SSEConnection struct {
  url string
  evChannel chan []byte
  workersActive int
}

func (self *SSEConnection) GetEventChannel() chan []byte {
  return self.evChannel
}

func (self *SSEConnection) parseChunk(chunk []byte) error {
  bi := bytes.NewBuffer(chunk)
  bo := bytes.NewBuffer([]byte(""))

  for {
    bx, err := bi.ReadBytes('\n')
    if err != nil {
      return err
    }

    bxt := bytes.TrimRight(bx, "\r\n")
    if len(bxt) == 0 {
      bob := bo.Bytes()
      //log.Printf("SSE  %s", bob)
      self.evChannel <- bob
      return nil
    }

    if !bytes.HasPrefix(bx, []byte("data: ")) {
      return errors.New("no data: prefix")
    }

    bo.Write(bx[6:len(bx)-1])
  }
}

func (self *SSEConnection) sseRetryWorker() error {
  time.Sleep(sRetryDelay)
  return self.sseWorker()
}

func (self *SSEConnection) workerStarted() {
  if self.workersActive != 0 {
    log.Panicf("excess workers (%d)", self.workersActive)
  }
  self.workersActive += 1
}

func (self *SSEConnection) workerExited() {
  self.workersActive -= 1
  if self.workersActive == 0 {
    go self.sseRetryWorker()
  }
}

func (self *SSEConnection) sseWorker() error {
  self.workerStarted()
  defer self.workerExited()

  conn, err := net.Dial("tcp", "bitcoinity.org:80")
  if err != nil {
    return err
  }


  conn.SetDeadline(time.Now().Add(sTimeout))
  /* /ev/markets/markets_$EXCHANGE_$CURRENCY?_=948734803280
   * EXCHANGE := mtgox / btcchina / btce / bitstamp / bitfinex
   *           / bitcurex / cavirtex / kraken / localbitcoins
   *           / campbx / rmbtb / justcoin / bit2c / bitquick / icbit
   * CURRENCY := USD / EUR / GBP / ...
   */

  /// XXX: URL currently hardcoded
  bw := bufio.NewWriterSize(conn, 512)
  fmt.Fprintf(bw, "GET /ev/markets/markets_bitstamp_USD?_=138 HTTP/1.1\r\n")
  fmt.Fprintf(bw, "Host: bitcoinity.org\r\n")
  fmt.Fprintf(bw, "User-Agent: Mozilla/5.0\r\n")
  fmt.Fprintf(bw, "Accept: */*\r\n")
  fmt.Fprintf(bw, "\r\n")
  err = bw.Flush()
  if err != nil {
    return err
  }

  br := bufio.NewReaderSize(conn, 512)
  state := sPreHeaders
  length := 0

  for {
    conn.SetDeadline(time.Now().Add(sTimeout))
    switch state {
      case sPreHeaders:
        s, err := br.ReadString('\n')
        if err != nil {
          return err
        }

        s  = strings.TrimRight(s, "\r\n")

        if len(s) == 0 {
          state = sLength
        }

      case sLength:
        s, err := br.ReadString('\n')
        if err != nil {
          return err
        }
        s  = strings.TrimRight(s, "\r\n")
        length_, err := strconv.ParseUint(s, 16, 32)
        if err != nil {
          return err
        }
        length = int(length_)
        state = sData

      case sData:
        b := make([]byte, length)
        n, err := io.ReadFull(br, b)
        if n != length {
          return errors.New("???")
        }
        if err != nil {
          return err
        }
        self.parseChunk(b)
        state = sPostChunk

      case sPostChunk:
        br.ReadString('\n')
        state = sLength
    }
  }

  return nil
}

func SSE(url string) *SSEConnection {
  ch := make(chan []byte)
  c := &SSEConnection{
    url: url,
    evChannel: ch,
    workersActive: 0,
  }

  go c.sseWorker()

  return c
}

// © 2013 Hugo Landau <hlandau@devever.net>    MIT License
