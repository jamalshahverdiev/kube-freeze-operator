package policy

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func matchLabelSelector(sel *metav1.LabelSelector, lbls map[string]string) (bool, error) {
	if sel == nil {
		return true, nil
	}
	s, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return false, err
	}
	return s.Matches(labels.Set(lbls)), nil
}
