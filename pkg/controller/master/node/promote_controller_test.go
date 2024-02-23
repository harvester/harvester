package node

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodeBuilder struct {
	node *corev1.Node
}

func NewDefaultNodeBuilder() *NodeBuilder {
	return &NodeBuilder{
		node: &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "",
				Labels:            map[string]string{},
				Annotations:       map[string]string{},
				CreationTimestamp: metav1.NewTime(time.Now()),
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{},
			},
		},
	}
}

func (n *NodeBuilder) Name(name string) *NodeBuilder {
	n.node.Name = name
	return n
}

func (n *NodeBuilder) Zone(name string) *NodeBuilder {
	n.node.Labels[corev1.LabelTopologyZone] = name
	return n
}

func (n *NodeBuilder) RoleMgmt() *NodeBuilder {
	n.node.Labels[HarvesterMgmtNodeLabelKey] = "true"
	return n
}

func (n *NodeBuilder) RoleWitness() *NodeBuilder {
	n.node.Labels[HarvesterWitnessNodeLabelKey] = "true"
	return n
}

func (n *NodeBuilder) RoleWorker() *NodeBuilder {
	n.node.Labels[HarvesterWorkerNodeLabelKey] = "true"
	return n
}

func (n *NodeBuilder) Harvester() *NodeBuilder {
	n.node.Labels[HarvesterManagedNodeLabelKey] = "true"
	return n
}

func (n *NodeBuilder) Witness() *corev1.Node {
	n.node.Labels[KubeEtcdNodeLabelKey] = "true"
	n.node.CreationTimestamp = metav1.NewTime(time.Now())
	return n.node
}

func (n *NodeBuilder) Management() *corev1.Node {
	n.node.Labels[KubeMasterNodeLabelKey] = "true"
	n.node.CreationTimestamp = metav1.NewTime(time.Now())
	return n.node
}

func (n *NodeBuilder) Worker() *corev1.Node {
	n.node.CreationTimestamp = metav1.NewTime(time.Now())
	return n.node
}

func (n *NodeBuilder) Running() *NodeBuilder {
	n.node.Annotations[HarvesterPromoteStatusAnnotationKey] = PromoteStatusRunning
	return n
}

func (n *NodeBuilder) Complete() *NodeBuilder {
	n.node.Annotations[HarvesterPromoteStatusAnnotationKey] = PromoteStatusComplete
	return n
}

func (n *NodeBuilder) Failed() *NodeBuilder {
	n.node.Annotations[HarvesterPromoteStatusAnnotationKey] = PromoteStatusFailed
	return n
}

func (n *NodeBuilder) Unknown() *NodeBuilder {
	n.node.Annotations[HarvesterPromoteStatusAnnotationKey] = PromoteStatusUnknown
	return n
}

func (n *NodeBuilder) NotReady() *NodeBuilder {
	ready := corev1.NodeCondition{
		Type:   corev1.NodeReady,
		Status: corev1.ConditionFalse,
	}
	n.node.Status.Conditions = append(n.node.Status.Conditions, ready)
	return n
}

var (
	// normal nodes
	mu1 = NewDefaultNodeBuilder().Name("m-unmanaged-1").Management()

	m1 = NewDefaultNodeBuilder().Name("m-1").Harvester().Management()
	m2 = NewDefaultNodeBuilder().Name("m-2").Harvester().Management()
	m3 = NewDefaultNodeBuilder().Name("m-3").Harvester().Management()

	witnc1 = NewDefaultNodeBuilder().Name("witness-c-1").Harvester().Complete().RoleWitness().Witness()
	witnr1 = NewDefaultNodeBuilder().Name("witness-r-1").Harvester().Running().Witness()
	witnf1 = NewDefaultNodeBuilder().Name("witness-f-1").Harvester().Failed().Witness()
	witnu1 = NewDefaultNodeBuilder().Name("witness-u-1").Harvester().Unknown().Witness()

	mc1 = NewDefaultNodeBuilder().Name("m-complete-1").Harvester().Complete().Management()

	wr1 = NewDefaultNodeBuilder().Name("w-running-1").Harvester().Running().Worker()
	wf1 = NewDefaultNodeBuilder().Name("w-failed-1").Harvester().Failed().Worker()

	wnr1 = NewDefaultNodeBuilder().Name("w-notready-1").Harvester().NotReady().Worker()
	wnr2 = NewDefaultNodeBuilder().Name("w-notready-2").Harvester().NotReady().Worker()

	wu1 = NewDefaultNodeBuilder().Name("w-unknown-1").Harvester().Unknown().Worker()

	wc1 = NewDefaultNodeBuilder().Name("w-complete-1").Harvester().Complete().Worker()

	w1 = NewDefaultNodeBuilder().Name("w-1").Harvester().Worker()
	w2 = NewDefaultNodeBuilder().Name("w-2").Harvester().Worker()
	w3 = NewDefaultNodeBuilder().Name("w-3").Harvester().Worker()

	w1rm  = NewDefaultNodeBuilder().Name("w-1-r-mgmt").Harvester().RoleMgmt().Worker()
	w1rwk = NewDefaultNodeBuilder().Name("w-1-r-worker").Harvester().RoleWorker().Worker()
	w1rw  = NewDefaultNodeBuilder().Name("w-1-r-witness").Harvester().RoleWitness().Worker()
	w2rw  = NewDefaultNodeBuilder().Name("w-2-r-witness").Harvester().RoleWitness().Worker()

	// zone aware nodes
	mu1z2 = NewDefaultNodeBuilder().Name("m-unmanaged-1-z2").Zone("zone2").Management()

	m1z1 = NewDefaultNodeBuilder().Name("m-1-z1").Zone("zone1").Harvester().Management()
	m2z2 = NewDefaultNodeBuilder().Name("m-2-z2").Zone("zone2").Harvester().Management()
	m3z3 = NewDefaultNodeBuilder().Name("m-3-z3").Zone("zone3").Harvester().Management()
	m4z1 = NewDefaultNodeBuilder().Name("m-4-z1").Zone("zone1").Harvester().Management()

	mc1z2 = NewDefaultNodeBuilder().Name("m-complete-1-z2").Zone("zone2").Harvester().Complete().Management()

	wr1z2 = NewDefaultNodeBuilder().Name("w-running-1-z2").Zone("zone2").Harvester().Running().Worker()
	wf1z2 = NewDefaultNodeBuilder().Name("w-failed-1-z2").Zone("zone2").Harvester().Failed().Worker()

	wnr1z2 = NewDefaultNodeBuilder().Name("w-notready-1-z2").Zone("zone2").Harvester().NotReady().Worker()
	wnr2z3 = NewDefaultNodeBuilder().Name("w-notready-2-z3").Zone("zone3").Harvester().NotReady().Worker()

	wu1z2 = NewDefaultNodeBuilder().Name("w-unknown-1-z2").Harvester().Unknown().Worker()

	wc1z2 = NewDefaultNodeBuilder().Name("w-complete-1-z2").Harvester().Complete().Worker()

	w1z1    = NewDefaultNodeBuilder().Name("w-1-z1").Zone("zone1").Harvester().Worker()
	w2z2    = NewDefaultNodeBuilder().Name("w-2-z2").Zone("zone2").Harvester().Worker()
	w3z3    = NewDefaultNodeBuilder().Name("w-3-z3").Zone("zone3").Harvester().Worker()
	w4z3    = NewDefaultNodeBuilder().Name("w-4-z3").Zone("zone3").Harvester().Worker()
	w5z2rm  = NewDefaultNodeBuilder().Name("w-5-z2-mgmt").Zone("zone2").Harvester().RoleMgmt().Worker()
	w6z2rwk = NewDefaultNodeBuilder().Name("w-6-z2-worker").Zone("zone2").Harvester().RoleWorker().Worker()
	w7z1    = NewDefaultNodeBuilder().Name("w-7-z1").Zone("zone1").Harvester().Worker()
)

func Test_selectPromoteNode(t *testing.T) {
	type args struct {
		nodeList []*corev1.Node
	}
	tests := []struct {
		name string
		args args
		want *corev1.Node
	}{
		{
			name: "one management",
			args: args{
				nodeList: []*corev1.Node{m1},
			},
			want: nil,
		},
		{
			name: "one management and one worker",
			args: args{
				nodeList: []*corev1.Node{m1, w1},
			},
			want: nil,
		},
		{
			name: "one management and two worker",
			args: args{
				nodeList: []*corev1.Node{m1, w1, w2},
			},
			want: w1,
		},
		{
			name: "one management and three worker",
			args: args{
				nodeList: []*corev1.Node{m1, w1, w2, w3},
			},
			want: w1,
		},
		{
			name: "one management and one not ready worker",
			args: args{
				nodeList: []*corev1.Node{m1, wnr1},
			},
			want: nil,
		},
		{
			name: "one management and one not ready worker and one ready worker",
			args: args{
				nodeList: []*corev1.Node{m1, wnr1, w1},
			},
			want: nil,
		},
		{
			name: "one management and one not ready worker and two ready worker",
			args: args{
				nodeList: []*corev1.Node{m1, wnr1, w1, w2},
			},
			want: w1,
		},
		{
			name: "one management and two not ready worker and one ready worker",
			args: args{
				nodeList: []*corev1.Node{m1, wnr1, wnr2, w1},
			},
			want: nil,
		},
		{
			name: "one management and two not ready worker and two ready worker",
			args: args{
				nodeList: []*corev1.Node{m1, wnr1, wnr2, w1, w2},
			},
			want: w1,
		},
		{
			name: "two management",
			args: args{
				nodeList: []*corev1.Node{m1, m2},
			},
			want: nil,
		},
		{
			name: "two management and one worker",
			args: args{
				nodeList: []*corev1.Node{m1, m2, w1},
			},
			want: w1,
		},
		{
			name: "two management and two worker",
			args: args{
				nodeList: []*corev1.Node{m1, m2, w1, w2},
			},
			want: w1,
		},
		{
			name: "two management and three worker",
			args: args{
				nodeList: []*corev1.Node{m1, m2, w1, w2, w3},
			},
			want: w1,
		},
		{
			name: "one management and one promoting worker",
			args: args{
				nodeList: []*corev1.Node{m1, wr1},
			},
			want: nil,
		},
		{
			name: "one management and one promoting worker and one worker",
			args: args{
				nodeList: []*corev1.Node{m1, wr1, w1},
			},
			want: nil,
		},
		{
			name: "one management and one promoting worker and two worker",
			args: args{
				nodeList: []*corev1.Node{m1, wr1, w1, w2},
			},
			want: nil,
		},
		{
			name: "one management and one promoting worker and three worker",
			args: args{
				nodeList: []*corev1.Node{m1, wr1, w1, w2, w3},
			},
			want: nil,
		},
		{
			name: "one management and one promote failed worker",
			args: args{
				nodeList: []*corev1.Node{m1, wf1},
			},
			want: nil,
		},
		{
			name: "one management and one promote failed worker and one worker",
			args: args{
				nodeList: []*corev1.Node{m1, wf1, w1},
			},
			want: nil,
		},
		{
			name: "one management and one promote failed worker and two worker",
			args: args{
				nodeList: []*corev1.Node{m1, wf1, w1, w2},
			},
			want: nil,
		},
		{
			name: "one management and one promote failed worker and three worker",
			args: args{
				nodeList: []*corev1.Node{m1, wf1, w1, w2, w3},
			},
			want: nil,
		},
		{
			name: "one management and one promoted management",
			args: args{
				nodeList: []*corev1.Node{m1, mc1},
			},
			want: nil,
		},
		{
			name: "one management and one promoted management and one worker",
			args: args{
				nodeList: []*corev1.Node{m1, mc1, w1},
			},
			want: w1,
		},
		{
			name: "one management and one promoted management and two worker",
			args: args{
				nodeList: []*corev1.Node{m1, mc1, w1, w2},
			},
			want: w1,
		},
		{
			name: "one management and one promoted management and three worker",
			args: args{
				nodeList: []*corev1.Node{m1, mc1, w1, w2, w3},
			},
			want: w1,
		},
		{
			name: "one managed management and one unmanaged management",
			args: args{
				nodeList: []*corev1.Node{m1, mu1},
			},
			want: nil,
		},
		{
			name: "one managed management and one unmanaged management and one worker",
			args: args{
				nodeList: []*corev1.Node{m1, mu1, w1},
			},
			want: w1,
		},
		{
			name: "one managed management and one unmanaged management and two worker",
			args: args{
				nodeList: []*corev1.Node{m1, mu1, w1, w2},
			},
			want: w1,
		},
		{
			name: "one managed management and one unmanaged management and three worker",
			args: args{
				nodeList: []*corev1.Node{m1, mu1, w1, w2, w3},
			},
			want: w1,
		},
		{
			name: "three management",
			args: args{
				nodeList: []*corev1.Node{m1, m2, m3},
			},
			want: nil,
		},
		{
			name: "three management and one worker",
			args: args{
				nodeList: []*corev1.Node{m1, m2, m3, w1},
			},
			want: nil,
		},
		{
			name: "one management and one promote unknown worker and one worker",
			args: args{
				nodeList: []*corev1.Node{m1, wu1, w1},
			},
			want: nil,
		},
		{
			name: "one management and one promoted worker and one worker",
			args: args{
				nodeList: []*corev1.Node{m1, wc1, w1},
			},
			want: nil,
		},
		{
			name: "one management in zone1",
			args: args{
				nodeList: []*corev1.Node{m1z1},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one worker in zone1",
			args: args{
				nodeList: []*corev1.Node{m1z1, w1z1},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and two worker in zone1",
			args: args{
				nodeList: []*corev1.Node{m1z1, w1z1, w7z1},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one worker in zone2",
			args: args{
				nodeList: []*corev1.Node{m1z1, w2z2, w3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and two worker",
			args: args{
				nodeList: []*corev1.Node{m1z1, w1, w2},
			},
			want: nil,
		},
		{
			name: "one management and one worker in zone1 and one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, w1z1, w3z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and two workers in zone2 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, w2z2, w3z3},
			},
			want: w2z2,
		},
		{
			name: "one management in zone1 and one not ready worker in zone2",
			args: args{
				nodeList: []*corev1.Node{m1z1, wnr1z2},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one not ready worker in zone2 and one ready worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wnr1z2, w3z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one not ready worker in zone2 and two ready worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wnr1z2, w3z3, w4z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one not ready worker in zone2 and two ready workers in zone2 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wnr1z2, w2z2, w3z3},
			},
			want: w2z2,
		},
		{
			name: "one management in zone1 and two not ready workers in zone2 and zone3 and one ready worker in zone2",
			args: args{
				nodeList: []*corev1.Node{m1z1, wnr1z2, wnr2z3, w2z2},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and two not ready workers in zone2 and zone3 and two ready workers in zone2 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wnr1z2, wnr2z3, w2z2, w3z3},
			},
			want: w2z2,
		},
		{
			name: "two management in zone1 and zone2 and one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, m2z2, w3z3},
			},
			want: w3z3,
		},
		{
			name: "two management in zone1 and zone2 and two worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, m2z2, w4z3, w3z3},
			},
			want: w3z3,
		},
		{
			name: "two management in zone1 and zone2 and one worker in zone1",
			args: args{
				nodeList: []*corev1.Node{m1z1, m2z2, w1z1},
			},
			want: nil,
		},
		{
			name: "two management in zone1 and zone2 and two workers in zone1 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, m2z2, w1z1, w3z3},
			},
			want: w3z3,
		},
		{
			name: "two management in zone1 and zone2 and three workers in zone1, zone2 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, m2z2, w1z1, w2z2, w3z3},
			},
			want: w3z3,
		},
		{
			name: "one management in zone1 and one promoting worker in zone2",
			args: args{
				nodeList: []*corev1.Node{m1z1, wr1z2},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one promoting worker in zone2 and one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wr1z2, w3z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one promoting worker in zone2 and two workers in zone2 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wr1z2, w2z2, w3z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one promote failed worker in zone2",
			args: args{
				nodeList: []*corev1.Node{m1z1, wf1z2},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one promote failed worker in zone2 and one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wf1z2, w3z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one promote failed worker in zone2 and two workers in zone2 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wf1z2, w2z2, w3z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one promoted management in zone2",
			args: args{
				nodeList: []*corev1.Node{m1z1, mc1z2},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one promoted management in zone2 and one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, mc1z2, w3z3},
			},
			want: w3z3,
		},
		{
			name: "one management in zone1 and one promoted management in zone2 and two workers in zone2 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, mc1z2, w2z2, w3z3},
			},
			want: w3z3,
		},
		{
			name: "one management in zone1 and one unmanaged management in zone2",
			args: args{
				nodeList: []*corev1.Node{m1z1, mu1z2},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one unmanaged management in zone2 and one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, mu1z2, w3z3},
			},
			want: w3z3,
		},
		{
			name: "one management in zone1 and one unmanaged management in zone2 and two workers in zone2 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, mu1z2, w2z2, w3z3},
			},
			want: w3z3,
		},
		{
			name: "three management in zone1, zone2 and zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, m2z2, m3z3},
			},
			want: nil,
		},
		{
			name: "three management in zone1, zone2 and zone3 and one worker in zone1",
			args: args{
				nodeList: []*corev1.Node{m1z1, m2z2, m3z3, w1z1},
			},
			want: nil,
		},
		{
			name: "three management in zone1, zone2 and zone1 (could be before labeling) and one worker in zone1",
			args: args{
				nodeList: []*corev1.Node{m1z1, m2z2, m4z1, w1z1},
			},
			want: nil,
		},
		{
			name: "three management in zone1, zone2 and zone1 (could be before labeling) and one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, m2z2, m4z1, w3z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one promote unknown in zone2 worker and one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wu1z2, w3z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1 and one promoted worker in zone2 and one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, wc1z2, w3z3},
			},
			want: nil,
		},
		{
			name: "two management one witness",
			args: args{
				nodeList: []*corev1.Node{m1, m2, witnc1},
			},
			want: nil,
		},
		{
			name: "one management two witness",
			args: args{
				nodeList: []*corev1.Node{m1, w1rw, w2rw},
			},
			want: nil,
		},
		{
			name: "one management one witness one worker",
			args: args{
				nodeList: []*corev1.Node{m1, w1rw, w2rw, w3},
			},
			want: w1rw,
		},
		{
			name: "one management one witness promoted one witness one worker",
			args: args{
				nodeList: []*corev1.Node{m1, witnc1, w2rw, w3},
			},
			want: w3,
		},
		{
			name: "one management one witness one promoted witness one worker order testing",
			args: args{
				nodeList: []*corev1.Node{m1, w2rw, witnc1, w3},
			},
			want: w3,
		},
		{
			name: "one management one running witness one witness one worker",
			args: args{
				nodeList: []*corev1.Node{m1, witnr1, w2rw, w3},
			},
			want: nil,
		},
		{
			name: "one management one failed witness one witness one worker",
			args: args{
				nodeList: []*corev1.Node{m1, witnf1, w2rw, w3},
			},
			want: nil,
		},
		{
			name: "one management one unknown witness one witness one worker",
			args: args{
				nodeList: []*corev1.Node{m1, witnu1, w2rw, w3},
			},
			want: nil,
		},
		{
			name: "two management one worker one worker with role management",
			args: args{
				nodeList: []*corev1.Node{m1, m2, w1, w1rm},
			},
			want: w1rm,
		},
		{
			name: "two management one worker one worker and one worker with role witness",
			args: args{
				nodeList: []*corev1.Node{m1, m2, w1, w1rw},
			},
			want: w1rw,
		},
		{
			name: "two management one worker with role worker",
			args: args{
				nodeList: []*corev1.Node{m1, m2, w1rwk},
			},
			want: nil,
		},
		{
			name: "one management, one management in zone1, one worker in zone2, one worker with role management in zone2, one worker with in zone3",
			args: args{
				nodeList: []*corev1.Node{m1, m1z1, w2z2, w5z2rm, w3z3},
			},
			want: w5z2rm,
		},
		{
			name: "one management in zone1, one worker with role worker in zone2, one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, w6z2rwk, w3z3},
			},
			want: nil,
		},
		{
			name: "one management in zone1, one worker with role worker in zone2, one worker in zone2, one worker in zone3",
			args: args{
				nodeList: []*corev1.Node{m1z1, w6z2rwk, w2z2, w3z3},
			},
			want: w2z2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectPromoteNode(tt.args.nodeList); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("selectPromoteNode() = %v, want %v", got, tt.want)
			}
		})
	}
}
