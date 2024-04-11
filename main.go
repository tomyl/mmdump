package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"
)

type pref struct {
	Category string `json:"category"`
	Name     string `json:"name"`
}

type user struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type file struct {
	ID        string `json:"id"`
	Extension string `json:"extension"`
}

type metadata struct {
	Files []file `json:"files"`
}

type post struct {
	CreateAt int64    `json:"create_at"`
	UserID   string   `json:"user_id"`
	Message  string   `json:"message"`
	Metadata metadata `json:"metadata"`
}

type postsEnvelope struct {
	Order      []string        `json:"order"`
	Posts      map[string]post `json:"posts"`
	PrevPostID string          `json:"prev_post_id"`
}

type channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

func get(endpoint, cookie, resource string, ignore404 bool) ([]byte, error) {
	u := endpoint + resource
	log.Printf("get %s", u)

	client := &http.Client{}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("cookie", cookie)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound && ignore404 {
			log.Printf("get %s: %s", u, resp.Status)
			return nil, nil
		}
		os.Stdout.Write(body)
		return nil, errors.New("not ok")
	}

	return body, nil
}

func getIfNotExists(endpoint, cookie, resource, dir, base string, ignore404 bool) (string, []byte, error) {
	name := filepath.Join(dir, base)

	if _, err := os.Stat(name); err == nil || !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return name, nil, nil
		}
		return "", nil, err
	}

	body, err := get(endpoint, cookie, resource, ignore404)
	if err != nil {
		return "", nil, err
	}
	if body == nil {
		return "", nil, nil
	}

	file, err := os.Create(name)
	if err != nil {
		return "", nil, err
	}

	defer file.Close()

	if _, err := file.Write(body); err != nil {
		return "", nil, err
	}

	return name, body, nil
}

func getCached(endpoint, cookie, resource, dir, base string, ignore404 bool) ([]byte, error) {
	name, body, err := getIfNotExists(endpoint, cookie, resource, dir, base, ignore404)
	if err != nil {
		return nil, err
	}

	if body != nil {
		return body, nil
	}

	return os.ReadFile(name)
}

func dumpPosts(endpoint, cookie, dir, channelID string) error {
	before := ""
	for {
		resource := fmt.Sprintf("channels/%s/posts?before=%s&per_page=1000", channelID, before)
		name := fmt.Sprintf("posts/%s_%s.json", channelID, before)
		body, err := getCached(endpoint, cookie, resource, dir, name, false)
		if err != nil {
			return err
		}

		var posts postsEnvelope
		if err := json.Unmarshal(body, &posts); err != nil {
			return err
		}

		for _, p := range posts.Posts {
			for _, f := range p.Metadata.Files {
				ext := ""
				if f.Extension != "" {
					ext = "." + f.Extension
				}
				resource := fmt.Sprintf("files/%s", f.ID)
				name := fmt.Sprintf("files/%s%s", f.ID, ext)
				if _, _, err := getIfNotExists(endpoint, cookie, resource, dir, name, true); err != nil {
					return err
				}
			}
		}

		if len(posts.Order) == 0 {
			break
		}

		before = posts.Order[len(posts.Order)-1]
	}

	return nil
}

func dumpChannel(endpoint, cookie, dir, channelID string) error {
	{
		resource := fmt.Sprintf("channels/%s", channelID)
		name := fmt.Sprintf("channels/%s.json", channelID)
		if _, _, err := getIfNotExists(endpoint, cookie, resource, dir, name, false); err != nil {
			return err
		}
	}
	{
		resource := fmt.Sprintf("channels/%s/members", channelID)
		name := fmt.Sprintf("channels/%s.members.json", channelID)
		if _, _, err := getIfNotExists(endpoint, cookie, resource, dir, name, false); err != nil {
			return err
		}
	}

	if err := dumpPosts(endpoint, cookie, dir, channelID); err != nil {
		return err
	}

	return nil
}

func dump(endpoint, cookie, dir, channelID string) error {
	_, _, err := getIfNotExists(endpoint, cookie, "users/me/preferences", dir, "preferences.json", false)
	if err != nil {
		return err
	}

	_, _, err = getIfNotExists(endpoint, cookie, "users?per_page=1000", dir, "users.json", false)
	if err != nil {
		return err
	}

	_, _, err = getIfNotExists(endpoint, cookie, "teams", dir, "teams.json", false)
	if err != nil {
		return err
	}

	channelsBody, err := getCached(endpoint, cookie, "users/me/channels?include_total_count=true&per_page=1000&include_deleted=true", dir, "channels.json", false)
	if err != nil {
		return err
	}

	var channels []channel
	if err := json.Unmarshal(channelsBody, &channels); err != nil {
		return err
	}

	for _, c := range channels {
		if channelID != "" && c.ID != channelID {
			continue
		}
		if err := dumpChannel(endpoint, cookie, dir, c.ID); err != nil {
			return err
		}
	}

	return nil
}

func listChannels(dir string) error {
	name := filepath.Join(dir, "channels.json")
	channelsBody, err := os.ReadFile(name)
	if err != nil {
		return err
	}

	var channels []channel
	if err := json.Unmarshal(channelsBody, &channels); err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintf(w, "ID\tDisplay name\n")

	for _, c := range channels {
		fmt.Fprintf(w, "%s\t%s\n", c.ID, c.DisplayName)
	}

	w.Flush()

	return nil
}

func listPostsBefore(dir, channelID, before string, userMap map[string]string, w io.Writer) error {
	name := filepath.Join(dir, "posts", fmt.Sprintf("%s_%s.json", channelID, before))
	postsBody, err := os.ReadFile(name)
	if err != nil {
		return err
	}

	var posts postsEnvelope
	if err := json.Unmarshal(postsBody, &posts); err != nil {
		return err
	}

	if len(posts.Order) > 0 {
		listPostsBefore(dir, channelID, posts.Order[len(posts.Order)-1], userMap, w)
	}

	for i := range posts.Order {
		postID := posts.Order[len(posts.Order)-i-1]
		p := posts.Posts[postID]
		t := time.Unix(p.CreateAt/1000, 0).Format(time.DateTime)
		u := userMap[p.UserID]
		fmt.Fprintf(w, "%s\t%s\t%s\n", t, u, p.Message)
	}

	return nil
}

func listPosts(dir, channelID string) error {
	name := filepath.Join(dir, "users.json")
	usersBody, err := os.ReadFile(name)
	if err != nil {
		return err
	}

	var users []user
	if err := json.Unmarshal(usersBody, &users); err != nil {
		return err
	}

	userMap := make(map[string]string, len(users))
	for _, u := range users {
		userMap[u.ID] = u.Username
	}

	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintf(w, "Time\tUser\tMessage\n")

	if err := listPostsBefore(dir, channelID, "", userMap, w); err != nil {
		return err
	}

	w.Flush()

	return nil
}

func run() error {
	doDump := flag.Bool("dump", false, "Dump data from Mattermost")
	doListChannels := flag.Bool("channels", false, "List channels")
	doListPosts := flag.String("posts", "", "List posts for provided channel ID")

	endpoint := flag.String("endpoint", "", "The API endpoint e.g. https://mattermost.example.com/api/v4/")
	cookie := flag.String("cookie", "", "Mattermost cookie")
	dir := flag.String("dir", "", "Output directory")
	channelID := flag.String("channel", "", "Dump only this channel ID")

	flag.Parse()

	if *doDump {
		if *endpoint == "" {
			return errors.New("-endpoint not provided")
		}

		if *cookie == "" {
			return errors.New("-cookie not provided")
		}

		if *dir == "" {
			return errors.New("-dir not provided")
		}

		for _, subdir := range []string{"channels", "files", "posts"} {
			if err := os.MkdirAll(filepath.Join(*dir, subdir), 0755); err != nil {
				return err
			}
		}

		return dump(*endpoint, *cookie, *dir, *channelID)
	} else if *doListChannels {
		if *dir == "" {
			return errors.New("-dir not provided")
		}
		return listChannels(*dir)
	} else if *doListPosts != "" {
		if *dir == "" {
			return errors.New("-dir not provided")
		}
		return listPosts(*dir, *doListPosts)
	} else {
		return errors.New("provide -dump or -channels")
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}
