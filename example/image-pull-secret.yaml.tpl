apiVersion: v1
data:
  .dockerconfigjson: "TODO: base64 encoded access token goes here"
kind: Secret
metadata:
  name: gardener-custom-metrics-image-pull-secret
  namespace: garden
type: kubernetes.io/dockerconfigjson
