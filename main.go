package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

//go:embed client.json
var data []byte

type clientInfo struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectUris []string `json:"redirect_uris"`
	AuthURI      string   `json:"auth_uri"`
	TokenURI     string   `json:"token_uri"`
}

type clientJSON struct {
	Web clientInfo `json:"web"`
}

func load() (clientInfo, error) {
	var cj clientJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		return clientInfo{}, err
	}
	return cj.Web, nil
}

func genState() (string, error) {
	state := make([]byte, 20)
	if _, err := io.ReadFull(rand.Reader, state); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(state), nil
	// return "state", nil
}

func recieve() <-chan string {
	ch := make(chan string)
	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		var (
			code  string
			state string
		)
		for k, u := range r.URL.Query() {
			switch k {
			case "code":
				code = u[0]
			case "state":
				state = u[0]
			}
		}
		ch <- state
		ch <- code
	})
	go http.ListenAndServe(":8080", nil)
	return ch
}

func main() {
	ci, err := load()
	if err != nil {
		panic(err)
	}
	conf := oauth2.Config{
		ClientID:     ci.ClientID,
		ClientSecret: ci.ClientSecret,
		RedirectURL:  ci.RedirectUris[0],
		Scopes: []string{
			calendar.CalendarReadonlyScope,
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  ci.AuthURI,
			TokenURL: ci.TokenURI,
		},
	}
	state, err := genState()
	if err != nil {
		panic(err)
	}
	// refresh tokenを取得するため、offlineを指定
	// ユーザーが一度承認をしたあとでも、再度承認画面を経るように、forceを指定
	url := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Printf("Visit the URL: %v\n", url)

	ch := recieve()
	recieveState := <-ch
	if recieveState != state {
		panic(fmt.Sprintf("invalid state. got=%v, want=%v", recieveState, state))
	}
	code := <-ch

	fmt.Println("exchange token for auth code")

	token, err := conf.Exchange(context.Background(), code)
	if err != nil {
		panic(err)
	}
	fmt.Println()
	fmt.Printf("AccessToken=%+v\n", token.AccessToken)
	fmt.Printf("TokenType=%+v\n", token.TokenType)
	fmt.Printf("RefreshToken=%+v\n", token.RefreshToken)
	fmt.Printf("Expiry=%+v\n", token.Expiry)

	// 以下アクセストークンを使った処理
	{
		option := option.WithHTTPClient(conf.Client(context.Background(), token))
		service, err := calendar.NewService(context.Background(), option)
		if err != nil {
			panic(err)
		}
		events, err := service.Events.List("primary").
			MaxResults(10).
			Do()
		if err != nil {
			panic(err)
		}
		for _, e := range events.Items {
			fmt.Printf("DateTime=%s, Summary=%s, Description=%s\n",
				e.Start.DateTime,
				e.Summary,
				e.Description,
			)
		}
	}
}
