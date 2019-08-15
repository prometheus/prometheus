package oss

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

//
// CreateLiveChannel    create a live-channel
//
// channelName  the name of the channel
// config       configuration of the channel
//
// CreateLiveChannelResult  the result of create live-channel
// error        nil if success, otherwise error
//
func (bucket Bucket) CreateLiveChannel(channelName string, config LiveChannelConfiguration) (CreateLiveChannelResult, error) {
	var out CreateLiveChannelResult

	bs, err := xml.Marshal(config)
	if err != nil {
		return out, err
	}

	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	params := map[string]interface{}{}
	params["live"] = nil
	resp, err := bucket.do("PUT", channelName, params, nil, buffer, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

//
// PutLiveChannelStatus Set the status of the live-channel: enabled/disabled
//
// channelName  the name of the channel
// status       enabled/disabled
//
// error        nil if success, otherwise error
//
func (bucket Bucket) PutLiveChannelStatus(channelName, status string) error {
	params := map[string]interface{}{}
	params["live"] = nil
	params["status"] = status

	resp, err := bucket.do("PUT", channelName, params, nil, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// PostVodPlaylist  create an playlist based on the specified playlist name, startTime and endTime
//
// channelName  the name of the channel
// playlistName the name of the playlist, must end with ".m3u8"
// startTime    the start time of the playlist
// endTime      the endtime of the playlist
//
// error        nil if success, otherwise error
//
func (bucket Bucket) PostVodPlaylist(channelName, playlistName string, startTime, endTime time.Time) error {
	params := map[string]interface{}{}
	params["vod"] = nil
	params["startTime"] = strconv.FormatInt(startTime.Unix(), 10)
	params["endTime"] = strconv.FormatInt(endTime.Unix(), 10)

	key := fmt.Sprintf("%s/%s", channelName, playlistName)
	resp, err := bucket.do("POST", key, params, nil, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// GetVodPlaylist  get the playlist based on the specified channelName, startTime and endTime
//
// channelName  the name of the channel
// startTime    the start time of the playlist
// endTime      the endtime of the playlist
//
// io.ReadCloser reader instance for reading data from response. It must be called close() after the usage and only valid when error is nil.
// error        nil if success, otherwise error
//
func (bucket Bucket) GetVodPlaylist(channelName string, startTime, endTime time.Time) (io.ReadCloser, error) {
	params := map[string]interface{}{}
	params["vod"] = nil
	params["startTime"] = strconv.FormatInt(startTime.Unix(), 10)
	params["endTime"] = strconv.FormatInt(endTime.Unix(), 10)

	resp, err := bucket.do("GET", channelName, params, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

//
// GetLiveChannelStat   Get the state of the live-channel
//
// channelName  the name of the channel
//
// LiveChannelStat  the state of the live-channel
// error        nil if success, otherwise error
//
func (bucket Bucket) GetLiveChannelStat(channelName string) (LiveChannelStat, error) {
	var out LiveChannelStat
	params := map[string]interface{}{}
	params["live"] = nil
	params["comp"] = "stat"

	resp, err := bucket.do("GET", channelName, params, nil, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

//
// GetLiveChannelInfo   Get the configuration info of the live-channel
//
// channelName  the name of the channel
//
// LiveChannelConfiguration the configuration info of the live-channel
// error        nil if success, otherwise error
//
func (bucket Bucket) GetLiveChannelInfo(channelName string) (LiveChannelConfiguration, error) {
	var out LiveChannelConfiguration
	params := map[string]interface{}{}
	params["live"] = nil

	resp, err := bucket.do("GET", channelName, params, nil, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

//
// GetLiveChannelHistory    Get push records of live-channel
//
// channelName  the name of the channel
//
// LiveChannelHistory   push records
// error        nil if success, otherwise error
//
func (bucket Bucket) GetLiveChannelHistory(channelName string) (LiveChannelHistory, error) {
	var out LiveChannelHistory
	params := map[string]interface{}{}
	params["live"] = nil
	params["comp"] = "history"

	resp, err := bucket.do("GET", channelName, params, nil, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

//
// ListLiveChannel  list the live-channels
//
// options  Prefix: filter by the name start with the value of "Prefix"
//          MaxKeys: the maximum count returned
//          Marker: cursor from which starting list
//
// ListLiveChannelResult    live-channel list
// error    nil if success, otherwise error
//
func (bucket Bucket) ListLiveChannel(options ...Option) (ListLiveChannelResult, error) {
	var out ListLiveChannelResult

	params, err := getRawParams(options)
	if err != nil {
		return out, err
	}

	params["live"] = nil

	resp, err := bucket.do("GET", "", params, nil, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

//
// DeleteLiveChannel    Delete the live-channel. When a client trying to stream the live-channel, the operation will fail. it will only delete the live-channel itself and the object generated by the live-channel will not be deleted.
//
// channelName  the name of the channel
//
// error        nil if success, otherwise error
//
func (bucket Bucket) DeleteLiveChannel(channelName string) error {
	params := map[string]interface{}{}
	params["live"] = nil

	if channelName == "" {
		return fmt.Errorf("invalid argument: channel name is empty")
	}

	resp, err := bucket.do("DELETE", channelName, params, nil, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkRespCode(resp.StatusCode, []int{http.StatusNoContent})
}

//
// SignRtmpURL  Generate a RTMP push-stream signature URL for the trusted user to push the RTMP stream to the live-channel.
//
// channelName  the name of the channel
// playlistName the name of the playlist, must end with ".m3u8"
// expires      expiration (in seconds)
//
// string       singed rtmp push stream url
// error        nil if success, otherwise error
//
func (bucket Bucket) SignRtmpURL(channelName, playlistName string, expires int64) (string, error) {
	if expires <= 0 {
		return "", fmt.Errorf("invalid argument: %d, expires must greater than 0", expires)
	}
	expiration := time.Now().Unix() + expires

	return bucket.Client.Conn.signRtmpURL(bucket.BucketName, channelName, playlistName, expiration), nil
}
