package main

import (
	"encoding/json"
	"bufio"
	"flag"
	"log"
	"math/rand"
	"net"
	"time"
	"io"
	"os/exec"
	"strconv"
	
	"game"
)

type Edax struct {
	bin string
	cmd *exec.Cmd
	cin io.WriteCloser
	cout io.ReadCloser
	scanner *bufio.Scanner
	my_color string
	op_color string
}

func NewEdax(edax_bin string, move_time int) (*Edax,error) {
	cmd := exec.Command(edax_bin, "-gtp", "-move-time", strconv.Itoa(move_time))
	cin,_ := cmd.StdinPipe()
	cout,_ := cmd.StdoutPipe()
	scanner := bufio.NewScanner(cout)

	e := &Edax {
		bin: edax_bin,
		cmd: cmd,
		cin: cin,
		cout: cout,
		scanner: scanner,
		my_color: "",
		op_color: "",
	}
	err := e.cmd.Start()
	if err != nil {
		return nil,err
	} else {
		return e,nil
	}
}

func (e *Edax) move(mv string) {
	// nboard protocol
	//msg := "move " + mv + "\n"
	msg := "play " + e.op_color + " " + mv + "\n"
	io.WriteString(e.cin, msg)
}

func (e *Edax) do_go() {
	msg := "genmove " + e.my_color + "\n"
	io.WriteString(e.cin, msg)
}

//gtp protocol
func (e *Edax) get_next_move() string {
	for {
		e.scanner.Scan()
		m := e.scanner.Bytes()
		if len(m) > 3 && string(m[:2]) == "= " {
			return string(m)[2:]
		}
		time.Sleep(100*time.Millisecond)
	}
}

func (e *Edax) clear_board() {
	io.WriteString(e.cin, "clear_board\n")
	e.my_color = ""
	e.op_color = ""
}

type Login struct {
	Message  string // LOGIN
	Userid   string
	Password string
}

type Message struct {
	Message string // READY, a6,A6, pass,PASS, LOGOUT
}

type GameState int
type GameMessage struct {
	Message     string // PLAY, RESULT
	Gameid      string
	StartTime   int64
	EndTime     int64
	Black       string
	BlackRating string
	White       string
	WhiteRating string
	Turn        string
	Position    string
	Moves       []string
	BoardSize   int
	Timeout     int
	State       string
}

type Game struct {
	Timeout    int
	Board      *game.Board //pointer to Board
	Boardlen   int
}

func send_login_msg(conn net.Conn, userid string, password string) error {
	l := Login{
		Message: "LOGIN",
		Userid: userid,
		Password: password,
	}
	b := str2json(l)
	conn.Write(b)
	return nil
}

func send_msg(conn net.Conn, msg string) error {
	r := Message{Message: msg}
	b := str2json(r)
	conn.Write(b)
	return nil
}

func wait_msg(bio *bufio.Reader) ([]byte,error) {
	return bio.ReadBytes('\n')
}

func msg_type(b []byte) string {
	var m Message
	json.Unmarshal(b, &m)
	return m.Message
}

func str2json(v any) []byte {
	b, _ := json.Marshal(v)
	b = append(b, byte('\n'))
	return b
}

func json2msg(b []byte) Message {
	var m Message
	json.Unmarshal(b, &m)
	return m
}

func mk_game(gm *GameMessage) *Game {
	sfen := gm.Position
	turn := 0 // black
	if gm.Turn == "white" {
		turn = 1
	}
	boardlen := gm.BoardSize
	b := game.NewBoardSFEN(boardlen, sfen)
	b.Turn = turn
	
	g := Game{
		Timeout: gm.Timeout,
		Board: b,
		Boardlen: boardlen,
	}
	return &g
}

func json2game(b []byte) *Game {
	var gm GameMessage
	json.Unmarshal(b, &gm)
	g := mk_game(&gm)
	return g
}

func json2gm(b []byte) *GameMessage {
	var gm GameMessage
	json.Unmarshal(b, &gm)
	return &gm
}

func main() {
	rand.Seed(time.Now().UnixNano())
	addr := flag.String("addr", "localhost:19714", "server IP address:port")
	userid := flag.String("userid", "player1", "userid")
	password := flag.String("password", "password", "password")
	edax_bin := flag.String("edax", "./edax", "path to edax binary")
	move_time := flag.Int("move_time", 55, "edax move-time option (sec)")
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Println("Dial error ln =", conn, " err =", err)
		return
	}

	err = send_login_msg(conn, *userid, *password)
	if err != nil {
		log.Println("Login failed err =", err)
		return
	}

	bio := bufio.NewReader(conn)

	e,err := NewEdax(*edax_bin, *move_time)
	if err != nil {
		log.Println("Edax binary not found: err =", err)
		return
	}
	for {
		b,err := wait_msg(bio)
		if err != nil {
			log.Println("wait_msg err =", err)
			break
		}
		switch msg_type(b) {
		case "PLAY":
			gm := json2gm(b)
			if e.my_color == "" {
				if gm.Black == *userid {
					e.my_color = "black"
					e.op_color = "white"
					
				} else {
					e.my_color = "white"
					e.op_color = "black"
				}
			}

			n_moves := len(gm.Moves)
			var move string
			if n_moves == 0 {
				e.do_go()
				move = e.get_next_move()
			} else {
				last_mv := gm.Moves[n_moves-1]
				e.move(last_mv)
				e.do_go()
				move = e.get_next_move()
			}
			send_msg(conn, move)

		case "ISREADY":
			e.clear_board()

			err = send_msg(conn, "READY")
			if err != nil {
				log.Println("Ready message failed err =", err)
				conn.Close()
				return
			}

		case "RESULT":
			gm := json2gm(b)
			st := time.Unix(gm.StartTime,0)
			et := time.Unix(gm.EndTime,0)
			log.Printf("gameid=%s StartTime=%s EndTime=%s Black=%s/%s White=%s/%s boardsize=%d Result=%s Moves=%v\n",
				gm.Gameid, st, et, gm.Black, gm.BlackRating, gm.White, gm.WhiteRating,
				gm.BoardSize, gm.State, gm.Moves)
			send_msg(conn, "RESULTOK")

		default:
			log.Println(string(b))
		}
	}

	send_msg(conn, "LOGOUT")
}

