package vm

import (
	"fmt"
	"testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetSCNameFromImgID(t *testing.T) {
	type input struct {
		imageId string
		images  []*harvesterv1.VirtualMachineImage
	}
	type output struct {
		scname string
		err    error
	}
	testcases := []struct {
		desc string
		in   input
		ex   output
	}{
		{
			desc: "all good",
			in: input{
				imageId: "default/test",
				images: []*harvesterv1.VirtualMachineImage{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "notthisone",
							Namespace: "default",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "barfoo",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "default",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "foobar",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "alsonotthisone",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "barfoo",
							},
						},
					},
				},
			},
			ex: output{
				scname: "foobar",
				err:    nil,
			},
		},
		{
			desc: "image not found",
			in: input{
				imageId: "default/test",
				images: []*harvesterv1.VirtualMachineImage{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "notthisone",
							Namespace: "default",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "barfoo",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "alsonotthisone",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "barfoo",
							},
						},
					},
				},
			},
			ex: output{
				scname: "",
				err:    fmt.Errorf("virtualmachineimages.harvesterhci.io \"test\" not found"),
			},
		},
		{
			desc: "image doesn't have annotation",
			in: input{
				imageId: "default/test",
				images: []*harvesterv1.VirtualMachineImage{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "test",
							Namespace:   "default",
							Annotations: map[string]string{},
						},
					},
				},
			},
			ex: output{
				scname: "",
				err:    fmt.Errorf("VMImage default/test does not have an annotation %s", util.AnnotationStorageClassName),
			},
		},
	}

	for _, tc := range testcases {
		var res output
		var clientset = fake.NewSimpleClientset()

		for _, img := range tc.in.images {
			err := clientset.Tracker().Add(img)
			assert.Nil(t, err, "failed creating mock resources")
		}

		var handler = &vmActionHandler{
			vmImageCache: fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		}

		res.scname, res.err = handler.getSCNameFromImgID(tc.in.imageId)

		assert.Equal(t, res.scname, tc.ex.scname, "case %q", tc.desc)
		if tc.ex.err != nil && res.err != nil {
			assert.Equal(t, res.err.Error(), tc.ex.err.Error(), "case %q", tc.desc)
		} else {
			assert.Equal(t, res.err, tc.ex.err, "case %q", tc.desc)
		}
	}
}
