package console

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// withDummyParent runs f in a new network namespace that contains a dummy link
// to use as the macvlan parent. It skips the test when the process is not
// allowed to create network namespaces.
func withDummyParent(t *testing.T, f func(parent netlink.Link)) {
	runtime.LockOSThread()

	orig, err := netns.Get()
	require.NoError(t, err)
	defer orig.Close() //nolint:errcheck

	// netns.New can fail after it has already switched namespaces, so register
	// the restore first. Unlock only after restoring; a locked thread is
	// discarded when the goroutine exits.
	defer func() {
		if cur, err := netns.Get(); err == nil {
			same := cur.Equal(orig)
			cur.Close() //nolint:errcheck
			if same {
				runtime.UnlockOSThread()
				return
			}
		}
		if err := netns.Set(orig); err != nil {
			t.Errorf("restore network namespace: %v", err)
			return
		}
		runtime.UnlockOSThread()
	}()

	ns, err := netns.New()
	if err != nil {
		t.Skipf("cannot create a network namespace: %v", err)
	}
	defer ns.Close() //nolint:errcheck

	dummy := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "dummy0"}}
	require.NoError(t, netlink.LinkAdd(dummy))
	require.NoError(t, netlink.LinkSetUp(dummy))
	parent, err := netlink.LinkByName("dummy0")
	require.NoError(t, err)

	f(parent)
}

func tempMacvlans(t *testing.T) []string {
	links, err := netlink.LinkList()
	require.NoError(t, err)
	var names []string
	for _, l := range links {
		if strings.HasPrefix(l.Attrs().Name, tempMacvlanPrefix) {
			names = append(names, l.Attrs().Name)
		}
	}
	return names
}

func TestGetVipThroughDHCPRemovesMacvlanOnDHCPError(t *testing.T) {
	withDummyParent(t, func(parent netlink.Link) {
		// No DHCP server answers on a dummy link, so the request times out.
		vip, err := getVipThroughDHCP(parent.Attrs().Name, "")
		assert.Error(t, err)
		assert.Nil(t, vip)
		assert.Empty(t, tempMacvlans(t))
	})
}

func TestCreateMacvlanRemovesLinkWhenSetUpFails(t *testing.T) {
	withDummyParent(t, func(parent netlink.Link) {
		// The kernel refuses to bring up a macvlan that has the parent's MAC address.
		l, err := createMacvlan(parent.Attrs().Name, parent.Attrs().HardwareAddr.String())
		assert.ErrorContains(t, err, "failed to set")
		assert.Nil(t, l)
		assert.Empty(t, tempMacvlans(t))
	})
}
