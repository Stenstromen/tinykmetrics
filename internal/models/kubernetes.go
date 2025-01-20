package models

type Pod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type PodList struct {
	Pods []Pod `json:"pods"`
}

type NamespaceList struct {
	Namespaces []string `json:"namespaces"`
}
