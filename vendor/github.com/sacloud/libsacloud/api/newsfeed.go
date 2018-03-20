package api

import (
	"encoding/json"
	"github.com/sacloud/libsacloud/sacloud"
)

// NewsFeedAPI フィード(障害/メンテナンス情報)API
type NewsFeedAPI struct {
	client *Client
}

// NewsFeedURL フィード取得URL
var NewsFeedURL = "https://secure.sakura.ad.jp/rss/sakuranews/getfeeds.php?service=cloud&format=json"

// NewNewsFeedAPI フィード(障害/メンテナンス情報)API
func NewNewsFeedAPI(client *Client) *NewsFeedAPI {
	return &NewsFeedAPI{
		client: client,
	}
}

// GetFeed フィード全件取得
func (api *NewsFeedAPI) GetFeed() ([]sacloud.NewsFeed, error) {

	var res = []sacloud.NewsFeed{}
	data, err := api.client.newRequest("GET", NewsFeedURL, nil)
	if err != nil {
		return res, err
	}

	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetFeedByURL 指定のURLを持つフィードを取得
func (api *NewsFeedAPI) GetFeedByURL(url string) (*sacloud.NewsFeed, error) {
	res, err := api.GetFeed()
	if err != nil {
		return nil, err
	}
	for _, r := range res {
		if r.URL == url {
			return &r, nil
		}
	}
	return nil, nil
}
