package cluster

import (
	"github.com/hashicorp/memberlist"
	"sort"
	"time"
)


var (
	mlist *memberlist.Memberlist
)

type Options struct {
	ClusterlistenPort     int
	ClusterPeers 		  []string
}


func Position()int{
	all := mlist.Members()
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name< all[j].Name
	})

	k := 0
	for _, n := range all {
		if n.Name == mlist.LocalNode().Name{
			break
		}
		k++
	}
	return k
}


func CreateCluster(o *Options) error{
	c := memberlist.DefaultLocalConfig()
	c.Name = time.Now().Format("2006-01-02 15:04:05")
	c.BindPort = o.ClusterlistenPort
	//创建gossip网络
	m, err := memberlist.Create(c)
	if err != nil {
		return err
	}
	mlist = m
	members := o.ClusterPeers
	if len(members) > 0 {
		_, err := mlist.Join(members)
		if err != nil {
			return err
		}
	}
	return nil
}