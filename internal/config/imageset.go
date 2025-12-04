package config

import "os"

// CreateImageSetConfig creates the imageset configuration file
func CreateImageSetConfig(configPath string) error {
	return CreateImageSetConfigWithVersion(configPath, "v2alpha1")
}

// CreateImageSetConfigWithVersion creates the imageset configuration file with specified API version
func CreateImageSetConfigWithVersion(configPath string, apiVersion string) error {
	// Default to v2alpha1 if not specified
	if apiVersion == "" {
		apiVersion = "v2alpha1"
	}
	
	configContent := `---
apiVersion: mirror.openshift.io/` + apiVersion + `
kind: ImageSetConfiguration
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.19
      packages:
        - name: local-storage-operator
          channels:
            - name: stable
              minVersion: 4.19.0-202510142112
              maxVersion: 4.19.0-202510142112
        - name: odf-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: odf-dependencies
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: cephcsi-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: mcg-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: ocs-client-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: ocs-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: odf-csi-addons-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: odf-prometheus-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: rook-ceph-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: recipe
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: cluster-logging
          channels:
            - name: stable-6.4
              minVersion: 6.4.0
              maxVersion: 6.4.0
        - name: loki-operator
          channels:
            - name: stable-6.4
              minVersion: 6.4.0
              maxVersion: 6.4.0
`

	return os.WriteFile(configPath, []byte(configContent), 0644)
}

// CreatePlatformConfig creates the platform configuration file for upload
func CreatePlatformConfig(path string) error {
	return CreatePlatformConfigWithVersion(path, "v2alpha1")
}

// CreatePlatformConfigWithVersion creates the platform configuration file with specified API version
func CreatePlatformConfigWithVersion(path string, apiVersion string) error {
	// Default to v2alpha1 if not specified
	if apiVersion == "" {
		apiVersion = "v2alpha1"
	}
	
	configContent := `---
apiVersion: mirror.openshift.io/` + apiVersion + `
kind: ImageSetConfiguration
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.19
      packages:
        - name: local-storage-operator
          channels:
            - name: stable
              minVersion: 4.19.0-202510142112
              maxVersion: 4.19.0-202510142112
        - name: odf-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: odf-dependencies
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: cephcsi-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: mcg-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: ocs-client-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: ocs-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: odf-csi-addons-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: odf-prometheus-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: rook-ceph-operator
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: recipe
          channels:
            - name: stable-4.19
              minVersion: 4.19.6-rhodf
              maxVersion: 4.19.6-rhodf
        - name: cluster-logging
          channels:
            - name: stable-6.4
              minVersion: 6.4.0
              maxVersion: 6.4.0
        - name: loki-operator
          channels:
            - name: stable-6.4
              minVersion: 6.4.0
              maxVersion: 6.4.0
`

	return os.WriteFile(path, []byte(configContent), 0644)
}
