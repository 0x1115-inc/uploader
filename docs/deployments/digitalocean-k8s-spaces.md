# Deploy to DigitalOcean Kubernetes (DOKS) with Spaces

This guide deploys `uploader` to DOKS and uses DigitalOcean Spaces for object storage.

## 1) Prerequisites

- A DigitalOcean Kubernetes cluster (DOKS)
- `kubectl` configured for the cluster
- A container image for this service published to a registry
- A DigitalOcean Space (bucket), for example `uploader-prod`
- A Spaces access key + secret key with access to that bucket

Notes:
- This service uses SQLite, so run with `replicas: 1`.
- SQLite DB is persisted with a PVC mounted at `/data`.

## 2) Build and push image

Use your registry/image tag.

```bash
docker build -t registry.example.com/uploader:v1.0.0 .
docker push registry.example.com/uploader:v1.0.0
```

## 3) Create namespace and secret

```bash
kubectl create namespace uploader

kubectl -n uploader create secret generic uploader-secrets \
  --from-literal=S3_ACCESS_KEY='<spaces-access-key>' \
  --from-literal=S3_SECRET_KEY='<spaces-secret-key>'
```

## 4) Apply Kubernetes resources

Save as `k8s/uploader-do.yaml` and apply it.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: uploader-data
  namespace: uploader
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: uploader
  namespace: uploader
spec:
  replicas: 1
  selector:
    matchLabels:
      app: uploader
  template:
    metadata:
      labels:
        app: uploader
    spec:
      containers:
        - name: uploader
          image: registry.example.com/uploader:v1.0.0
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
          env:
            - name: PORT
              value: "8080"
            - name: DB_PATH
              value: "/data/uploader.db"
            - name: MAX_UPLOAD_MB
              value: "50"
            - name: LOG_LEVEL
              value: "info"
            - name: S3_ENDPOINT
              value: "nyc3.digitaloceanspaces.com"
            - name: S3_REGION
              value: "nyc3"
            - name: S3_BUCKET
              value: "uploader-prod"
            - name: S3_USE_SSL
              value: "true"
            - name: S3_ACCESS_KEY
              valueFrom:
                secretKeyRef:
                  name: uploader-secrets
                  key: S3_ACCESS_KEY
            - name: S3_SECRET_KEY
              valueFrom:
                secretKeyRef:
                  name: uploader-secrets
                  key: S3_SECRET_KEY
          volumeMounts:
            - name: data
              mountPath: /data
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: uploader-data
---
apiVersion: v1
kind: Service
metadata:
  name: uploader
  namespace: uploader
spec:
  selector:
    app: uploader
  ports:
    - port: 80
      targetPort: 8080
      protocol: TCP
  type: LoadBalancer
```

Apply:

```bash
kubectl apply -f k8s/uploader-do.yaml
```

## 5) Get endpoint and test

```bash
kubectl -n uploader get svc uploader
```

Use the external IP from `EXTERNAL-IP`:

```bash
curl -s -X POST http://<external-ip>/v1/files -F "file=@./test.pdf"
curl -v http://<external-ip>/v1/files/<file_id>/download
```

## 6) Troubleshooting

```bash
kubectl -n uploader get pods
kubectl -n uploader logs deploy/uploader --tail=200 -f
```

Common issues:
- `upload failed`: check Spaces credentials, bucket name, and `S3_ENDPOINT`/`S3_REGION`.
- `413 file exceeds max size`: file is above configured `MAX_UPLOAD_MB`.
- Pod restart loop: verify PVC is bound and writable.
