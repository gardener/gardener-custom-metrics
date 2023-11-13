pkg directory:
```
├── input - Takes care of source data: tracks seed's k8s contents, scrapes ShootKapis
│   ├── controller
│   │   ├── ...
│   │   ├── pod
│   │   │   └── ...
│   │   └── secret
│   │       └── ...
│   ├── input_data_registry - Repository for the metrics source data
│   │   └── input_data_registry.go
│   └── input_data_service.go - Primary responsible for providing input data
└── metrics_provider_service - Serves k8s metrics via HTTP
    ├── metrics_provider.go - Implements the provider interface required by the metrics server library
    └── metrics_provider_service.go - Primary responsible for serving K8s metrics
```