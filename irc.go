package main
import "net"
import "bufio"
import "fmt"
import "strings"
import "sync"
import "log"

type Connection struct {
  server   string
  nick     string
  user     string
  realName string

  nickservPassword string

  socket       net.Conn
  br           *bufio.Reader
  bw           *bufio.Writer

  eventChannel chan *Event
  writerChannel chan string
  writerSpawnOnce sync.Once
}

type Event struct {
  Raw      string    // :the.server.name 001 foo bar :blah blah blah
                     // :nick!user@host ...
  Source   string    // the.server.name
                     // nick!user@host
  SNick    string    // nick
  SUser    string    // user
  SHost    string    // host
  Command  string    // 001; NOTIFY
  Args     []string  // foo bar
  Message  string    // blah blah blah

  Handled  bool
}

func parseMessage(msg string) *Event {
  ev := &Event{Raw: msg}
  // :SERVERNAME 001 NAME :Welcome to the NETWORKNAME Internet Relay Chat Network NAME
  if msg[0] == ':' {
    if i := strings.Index(msg, " "); i >= -1 {
      ev.Source = msg[1:i]
      msg = msg[i+1:len(msg)]

      if i, j := strings.Index(ev.Source, "!"), strings.Index(ev.Source, "@"); i > -1 && j > -1 {
        ev.SNick = ev.Source[0:i]
        ev.SUser = ev.Source[i+1:j]
        ev.SHost = ev.Source[j+1:len(ev.Source)]
      }
    } else {
      // malformed message
      return nil
    }
  }

  args := strings.SplitN(msg, " :", 2)
  if len(args) > 1 {
    ev.Message = args[1]
  }

  args = strings.Split(args[0], " ")
  ev.Command = strings.ToUpper(args[0])
  if len(args) > 1 {
    ev.Args = args[1:len(args)]
  } else {
    ev.Args = []string{}
  }

  return ev
}

func (self *Connection) handleEvent(ev *Event) error {
  if (ev.Command == "001") {
    self.writerSpawnOnce.Do(func() {
      go self.writerMain()
    })
  } else if (ev.Command == "PING") {
    if (len(ev.Message) > 0) {
      // some ircds expect a successful pong response before
      // sending 001, so we do this directly.
      self.bw.WriteString(fmt.Sprintf("PONG :%s\n", ev.Message))
    } else {
      self.bw.WriteString(fmt.Sprintf("PONG\n"))
    }

    err := self.bw.Flush()
    if err != nil {
      return err
    }
  } else if (ev.Command == "NOTICE" &&
      ev.SNick == "NickServ" &&
      strings.HasPrefix(ev.Message, "This nickname is registered") &&
      len(self.nickservPassword) > 0) {
    self.SendLine(fmt.Sprintf("PRIVMSG NickServ :IDENTIFY %s", self.nickservPassword))
  }

  return nil
}

func (self *Connection) loop() error {
  for {
    msg, err := self.br.ReadString('\n')
    if err != nil {
      return err
    }

    ev := parseMessage(msg)
    err = self.handleEvent(ev)
    if err != nil {
      return err
    }

    if !ev.Handled {
      self.eventChannel <- ev
    }
  }
  return nil
}

func (self *Connection) writerMain() {
  for {
    s := <-self.writerChannel
    self.bw.WriteString(s)
    self.bw.WriteString("\n")
    err := self.bw.Flush()
    if err != nil {
    }
  }
}

func (self *Connection) main() error {
  socket, err := net.Dial("tcp", self.server)
  if err != nil {
    return err
  }
  self.socket = socket

  self.br = bufio.NewReaderSize(self.socket, 512)
  self.bw = bufio.NewWriterSize(self.socket, 512)

  // Opening sequence.
  fmt.Fprintf(self.bw, "NICK %s\nUSER %s - - %s\n", self.nick,
    self.user, self.realName)
  err = self.bw.Flush()
  if err != nil {
    return err
  }

  return self.loop()
}

func (self *Connection) GetEventChannel() chan *Event {
  return self.eventChannel
}

func (self *Connection) SendLine(msg string) {
  log.Printf("Sending: %s", msg)
  self.writerChannel <- msg
}

func (self *Connection) JoinChannel(channelName string) {
  self.SendLine(fmt.Sprintf("JOIN %s", channelName))
}

func (self *Connection) Msg(target, message string) {
  self.SendLine(fmt.Sprintf("PRIVMSG %s :%s", target, message))
}

func IRC(server, nick, user, realName, nickservPassword string) *Connection {
  ev_c := make(chan *Event, 1024)
  wr_c := make(chan string, 1024)
  irc := &Connection {
    server: server,
    nick: nick,
    user: user,
    realName: realName,
    nickservPassword: nickservPassword,
    eventChannel: ev_c,
    writerChannel: wr_c,
  }

  go irc.main()
  return irc
}

// © 2013 Hugo Landau <hlandau@devever.net>    MIT License
