// Package hashrate is a message type for Simplebuffers generated by miners to
// broadcast an IP address, a count and version number and current height
// of mining work just completed. This data should be stored in a log file and
// added together to generate hashrate reporting in nodes when their controller
// is running
package hashrate

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/p9c/pkg/app/slog"
	"io"
	"net"
	"time"

	"github.com/p9c/pod/pkg/coding/simplebuffer"
	"github.com/p9c/pod/pkg/coding/simplebuffer/IPs"
	"github.com/p9c/pod/pkg/coding/simplebuffer/Int32"
	"github.com/p9c/pod/pkg/coding/simplebuffer/Time"
)

var HashrateMagic = []byte{'h', 'a', 's', 'h'}

type Container struct {
	simplebuffer.Container
}

type Hashrate struct {
	Time    time.Time
	IPs     []*net.IP
	Count   int
	Version int32
	Height  int32
	Nonce   int32
}

func Get(count int32, version int32, height int32) Container {
	nonce := make([]byte, 4)
	if _, err := io.ReadFull(rand.Reader, nonce); slog.Check(err) {
	}
	return Container{*simplebuffer.Serializers{
		Time.New().Put(time.Now()),
		IPs.GetListenable(),
		Int32.New().Put(count),
		Int32.New().Put(version),
		Int32.New().Put(height),
		Int32.New().Put(int32(binary.BigEndian.Uint32(nonce))),
	}.CreateContainer(HashrateMagic)}
}

// LoadContainer takes a message byte slice payload and loads it into a container
// ready to be decoded
func LoadContainer(b []byte) (out Container) {
	out.Data = b
	return
}

func (j *Container) GetTime() time.Time {
	return Time.New().DecodeOne(j.Get(0)).Get()
}

func (j *Container) GetIPs() []*net.IP {
	return IPs.New().DecodeOne(j.Get(1)).Get()
}

func (j *Container) GetCount() int {
	return int(Int32.New().DecodeOne(j.Get(2)).Get())
}

func (j *Container) GetVersion() int32 {
	return Int32.New().DecodeOne(j.Get(3)).Get()
}

func (j *Container) GetHeight() int32 {
	return Int32.New().DecodeOne(j.Get(4)).Get()
}

func (j *Container) GetNonce() int32 {
	return Int32.New().DecodeOne(j.Get(5)).Get()
}

func (j *Container) String() (s string) {
	s += fmt.Sprint("\ntype '"+string(HashrateMagic)+"' elements:", j.Count())
	s += "\n"
	t := j.GetTime()
	s += "1 Time: "
	s += fmt.Sprint(t)
	s += "\n"
	ips := j.GetIPs()
	s += "2 IPs:"
	for i := range ips {
		s += fmt.Sprint(" ", ips[i].String())
	}
	s += "\n"
	count := j.GetCount()
	s += "3 Count: "
	s += fmt.Sprint(count)
	s += "\n"
	version := j.GetVersion()
	s += "4 Version: "
	s += fmt.Sprint(version)
	s += "\n"
	return
}

// Struct deserializes the data all in one go by calling the field deserializing
// functions into a structure containing the fields.
// The height is given in this report as it is part of the job message
// and makes it faster for clients to look up the algorithm name according to the
// block height, which can change between hard fork versions
func (j *Container) Struct() (out Hashrate) {
	out = Hashrate{
		Time:    j.GetTime(),
		IPs:     j.GetIPs(),
		Count:   j.GetCount(),
		Version: j.GetVersion(),
		Height:  j.GetHeight(),
		Nonce:   j.GetNonce(),
	}
	return
}
