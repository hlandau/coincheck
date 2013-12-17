package main
import "log"
import "encoding/json"
import "fmt"
import "io/ioutil"

var lastLastMap map[string]float64 = make(map[string]float64)

type BitcoinityMessage struct {
  Id       int
  Channel  string
  Text     map[string]interface{}
}

func logger(c *Connection) {
  ch := c.GetEventChannel()
  for {
    ev := <-ch
    log.Printf("ev  %+v", ev)
  }
}

func formatExchangeName(exName string) string {
  switch exName {
    case "kraken":   return "Kraken"
    case "bitstamp": return "Bitstamp"
    case "mtgox":    return "MtGox"
    default:         return exName
  }
}

func formatSummary(msg *BitcoinityMessage) string {
  if msg.Channel != "markets" {
    return ""
  }
  exName_, ok := msg.Text["exchange_name"]
  if !ok {
    return ""
  }
  exName := exName_.(string)

  currency, ok := msg.Text["currency"]
  if !ok {
    return ""
  }
  if currency != "USD" {
    return ""
  }

  last_, ok := msg.Text["last"]
  if !ok {
    return ""
  }
  last := last_.(float64)

  cols := ""
  lastLast, ok := lastLastMap[exName]
  if ok {
    if last > lastLast {
      cols = "\x03"+"3"
    } else if (last < lastLast) {
      cols = "\x03"+"4"
    }
  }

  lastLastMap[exName] = last

  exName = formatExchangeName(exName)
  return fmt.Sprintf(">%13s<%s>    Last: %s$%5.2f", exName, currency, cols, last)
}

type Config struct {
  IRC_Server   string `json:"irc_server"`
  IRC_Username string `json:"irc_username"`
  IRC_Realname string `json:"irc_realname"`
  IRC_Nickname string `json:"irc_nickname"`
  IRC_Password string `json:"irc_password"`
  IRC_Channel  string `json:"irc_channel"`
}

var config Config

func load_config() error {
  d, err := ioutil.ReadFile("ticker.json")
  if err != nil {
    return err
  }

  err = json.Unmarshal(d, &config)
  return err
}

func main() {
  err := load_config()
  if err != nil {
    log.Printf("error loading config: %+v", err)
    return
  }

  irc_c := IRC(config.IRC_Server, config.IRC_Nickname,
    config.IRC_Username, config.IRC_Realname,
    config.IRC_Password)
  irc_c.JoinChannel(config.IRC_Channel)

  go logger(irc_c)

  sse_c := SSE("http://bitcoinity.org/ev/markets/markets_bitstamp_USD?_=138643020")
  sse_ch := sse_c.GetEventChannel()

  /*
    id: #
    channel: markets
    text:
      exchange_name: btce
      currency: USD
      last: 742.01

    channel: markets_bitstamp_USD
    text:
      trade:
        amount: 9.3283289
        price: 746.0
        exchange_name: bitstamp
        date: 283032803

  */
  for {
    var tmsg BitcoinityMessage
    msg_b := <-sse_ch
    err = json.Unmarshal(msg_b, &tmsg)
    if err != nil {
      log.Printf("error: %+v (%v)", err, string(msg_b))
      continue
    }

    log.Printf("tm  %+v", tmsg)
    summaryMsg := formatSummary(&tmsg)
    if len(summaryMsg) > 0 {
      irc_c.Msg(config.IRC_Channel, summaryMsg)
    }
  }
}

// © 2013 Hugo Landau <hlandau@devever.net>    MIT License
