package main

import (
	"encoding/json"
	"bufio"
	"flag"
	"log"
	"math/rand"
	"net"
	"time"

	"game"
)

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
	Gameid     string
	StartTime  int64
	EndTime    int64
	Black      string
	White      string
	Moves      []string
	Timeout    int
	Board      *game.Board //pointer to Board
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
		Gameid: gm.Gameid,
		StartTime: gm.StartTime,
		EndTime: gm.EndTime,
		Black: gm.Black,
		White: gm.White,
		Moves: gm.Moves,
		Timeout: gm.Timeout,
		Board: b,
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
	sleep := flag.Bool("sleep", false, "sleeps for 11000msec")
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

	var g *Game
	for {
		b,err := wait_msg(bio)

		if *sleep == true && rand.Int() % 1000 == 0 {
			sleep_time := 9800 // rand.Int() % 4000
			time.Sleep(time.Duration(sleep_time)*time.Millisecond)
		}

		// log.Println("received msg = ", string(b))
		if err != nil {
			log.Println("wait_msg err =", err)
			break
		}
		switch msg_type(b) {
		case "PLAY":
			g = json2game(b)
			//g.Board.printBoard()
			var move string
			lms := g.Board.LegalMoves()
			if len(lms) == 0 {
				move = "pass"
			} else {
				pos := lms[rand.Int() % len(lms)]
				move = g.Board.Position2Str(pos)
			}
			send_msg(conn, move)

		case "ISREADY":
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

