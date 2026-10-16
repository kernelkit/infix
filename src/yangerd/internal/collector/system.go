package collector

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/numconv"
)

var platformKeyMap = map[string]string{
	"NAME":         "os-name",
	"VERSION_ID":   "os-version",
	"BUILD_ID":     "os-release",
	"ARCHITECTURE": "machine",
}

// servicesTimeout bounds one initctl call on the request path.
const servicesTimeout = 5 * time.Second

// Services reads the service list from finit when asked.  Finit has no
// event for a service changing state yet, so a GET asks it rather than
// a timer; initctl is a cheap local query.  The last good answer stands
// in while initctl cannot answer, e.g. during a reload, so a GET at the
// wrong moment does not lose the whole list.
type Services struct {
	cmd  CommandRunner
	mu   sync.Mutex
	last json.RawMessage
}

// NewServices creates a Services provider reading through cmd.
func NewServices(cmd CommandRunner) *Services {
	return &Services{cmd: cmd}
}

// Live is the infix-system:services overlay for ietf-system:system-state.
func (s *Services) Live() json.RawMessage {
	ctx, cancel := context.WithTimeout(context.Background(), servicesTimeout)
	defer cancel()

	data, err := servicesOverlay(ctx, s.cmd)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		return s.last
	}
	s.last = data
	return data
}

func servicesOverlay(ctx context.Context, cmd CommandRunner) (json.RawMessage, error) {
	var initData []map[string]interface{}
	if err := runJSON(ctx, cmd, &initData, "initctl", "-j"); err != nil {
		return nil, err
	}

	var services []interface{}
	for _, d := range initData {
		pid, ok := d["pid"]
		if !ok {
			continue
		}
		identity, ok := d["identity"]
		if !ok {
			continue
		}
		svc := map[string]interface{}{
			"pid":         toInt(pid),
			"name":        identity,
			"status":      d["status"],
			"description": d["description"],
			"statistics": map[string]interface{}{
				"memory-usage":  strconv.Itoa(toInt(zeroIfNil(d["memory"]))),
				"uptime":        strconv.Itoa(toInt(zeroIfNil(d["uptime"]))),
				"restart-count": toInt(zeroIfNil(d["restarts"])),
			},
		}
		services = append(services, svc)
	}

	return json.Marshal(map[string]interface{}{
		"infix-system:services": map[string]interface{}{
			"service": services,
		},
	})
}

func yangDateTime(t time.Time) string {
	return t.Format("2006-01-02T15:04:05-07:00")
}

func toInt(v interface{}) int {
	return numconv.IntOrZero(v)
}

func zeroIfNil(v interface{}) interface{} {
	if v == nil {
		return 0
	}
	return v
}
