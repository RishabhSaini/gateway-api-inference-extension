#!/bin/bash
# Copyright 2025 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# Configuration for CKS cluster and quay.io
REGISTRY="quay.io/rh_ee_rsaini"
TRAINING_IMAGE="latency-training"
PREDICTION_IMAGE="latency-prediction"
TEST_IMAGE="latency-test"
TAG="slo-experimental"
NAMESPACE="llm-d-pd"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

echo_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

echo_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if required files exist
check_files() {
    echo_status "Checking required files..."

    local files=("training_server.py" "prediction_server.py" "requirements.txt" "Dockerfile-training" "Dockerfile-prediction")
    for file in "${files[@]}"; do
        if [[ ! -f "$file" ]]; then
            echo_error "Required file $file not found!"
            exit 1
        fi
    done

    # Check for test-specific files
    local test_files=("Dockerfile-test" "test_dual_server_client.py")
    for file in "${test_files[@]}"; do
        if [[ ! -f "$file" ]]; then
            echo_warning "Test file $file not found - test image will not be built"
            TEST_BUILD_ENABLED=false
            return
        fi
    done

    TEST_BUILD_ENABLED=true
    echo_status "All required files found (including test files)."
}

# Build Docker images
build_images() {
    echo_status "Building Docker images for quay.io..."

    # Build training server image
    echo_status "Building training server image..."
    docker build -f Dockerfile-training -t ${REGISTRY}/${TRAINING_IMAGE}:${TAG} .

    # Build prediction server image
    echo_status "Building prediction server image..."
    docker build -f Dockerfile-prediction -t ${REGISTRY}/${PREDICTION_IMAGE}:${TAG} .

    # Build test image if enabled
    if [[ "$TEST_BUILD_ENABLED" == "true" ]]; then
        echo_status "Building test image..."
        docker build -f Dockerfile-test -t ${REGISTRY}/${TEST_IMAGE}:${TAG} .

        echo_status "All images (including test) built successfully."
    else
        echo_status "Images built successfully (test image skipped)."
    fi
}

# Push images to quay.io
push_images() {
    echo_status "Pushing images to quay.io..."

    # Check if logged in to quay.io
    if ! docker info | grep -q "Registry: quay.io"; then
        echo_warning "You may need to login to quay.io first:"
        echo "  docker login quay.io"
    fi

    # Push training server
    echo_status "Pushing training server image..."
    docker push ${REGISTRY}/${TRAINING_IMAGE}:${TAG}

    # Push prediction server
    echo_status "Pushing prediction server image..."
    docker push ${REGISTRY}/${PREDICTION_IMAGE}:${TAG}

    # Push test image if it exists
    if [[ "$TEST_BUILD_ENABLED" == "true" ]]; then
        echo_status "Pushing test image..."
        docker push ${REGISTRY}/${TEST_IMAGE}:${TAG}

        echo_status "All images (including test) pushed successfully."
    else
        echo_status "Images pushed successfully (test image skipped)."
    fi
}

# Deploy to CKS cluster
deploy_to_cluster() {
    echo_status "Deploying to CKS cluster (namespace: ${NAMESPACE})..."

    # Check if namespace exists
    if ! kubectl get namespace ${NAMESPACE} &> /dev/null; then
        echo_status "Creating namespace ${NAMESPACE}..."
        kubectl create namespace ${NAMESPACE}
    fi

    # Apply the deployment
    if [[ -f "manifests/dual-server-deployment.yaml" ]]; then
        echo_status "Applying dual-server deployment..."
        kubectl apply -f manifests/dual-server-deployment.yaml

        echo_status "Deployment applied successfully."
        echo_status "Checking deployment status..."
        kubectl get pods -n ${NAMESPACE} -l component=training
        kubectl get pods -n ${NAMESPACE} -l component=prediction
    else
        echo_error "manifests/dual-server-deployment.yaml not found!"
        exit 1
    fi
}

# Deploy test job
deploy_test() {
    echo_status "Deploying test job to CKS cluster..."

    # Check if namespace exists
    if ! kubectl get namespace ${NAMESPACE} &> /dev/null; then
        echo_error "Namespace ${NAMESPACE} does not exist. Deploy main services first."
        exit 1
    fi

    # Delete existing test jobs
    echo_status "Cleaning up old test jobs..."
    kubectl delete job latency-predictor-test -n ${NAMESPACE} --ignore-not-found=true

    # Apply the test deployment
    if [[ -f "manifests/test-dual-server-deployment.yaml" ]]; then
        echo_status "Applying test job..."
        kubectl apply -f manifests/test-dual-server-deployment.yaml

        echo_status "Test job created successfully."
        echo_status "Monitor test progress with:"
        echo "  kubectl logs -n ${NAMESPACE} -l app=latency-predictor-test --follow"
    else
        echo_error "manifests/test-dual-server-deployment.yaml not found!"
        exit 1
    fi
}

# Watch test logs
watch_test_logs() {
    echo_status "Waiting for test pod to start..."
    kubectl wait --for=condition=ready pod -l app=latency-predictor-test -n ${NAMESPACE} --timeout=60s || true

    echo_status "Streaming test logs..."
    kubectl logs -n ${NAMESPACE} -l app=latency-predictor-test --follow
}

# Get test results
get_test_results() {
    echo_status "Getting test results..."

    # Get test job status
    kubectl get jobs -n ${NAMESPACE} -l app=latency-predictor-test

    # Get pod logs
    echo ""
    echo_status "Test logs:"
    kubectl logs -n ${NAMESPACE} -l app=latency-predictor-test --tail=100
}

# Show deployment info
show_info() {
    echo_status "Deployment Information:"
    echo ""
    echo "Registry: ${REGISTRY}"
    echo "Tag: ${TAG}"
    echo "Namespace: ${NAMESPACE}"
    echo ""
    echo "Images:"
    echo "  Training: ${REGISTRY}/${TRAINING_IMAGE}:${TAG}"
    echo "  Prediction: ${REGISTRY}/${PREDICTION_IMAGE}:${TAG}"
    echo "  Test: ${REGISTRY}/${TEST_IMAGE}:${TAG}"
    echo ""
    echo "Services in cluster:"
    kubectl get svc -n ${NAMESPACE} 2>/dev/null || echo "  (namespace not found)"
    echo ""
    echo "Pods in cluster:"
    kubectl get pods -n ${NAMESPACE} 2>/dev/null || echo "  (namespace not found)"
}

# List built images
list_images() {
    echo_status "Local Docker images:"
    docker images | grep -E "(${REGISTRY}|latency)" | grep "${TAG}" || echo "No images found with tag ${TAG}"
}

# Main script logic
case "${1}" in
    check)
        check_files
        ;;
    build)
        check_files
        build_images
        ;;
    push)
        push_images
        ;;
    deploy)
        deploy_to_cluster
        ;;
    test-deploy)
        deploy_test
        ;;
    test-logs)
        watch_test_logs
        ;;
    test-results)
        get_test_results
        ;;
    info)
        show_info
        ;;
    images)
        list_images
        ;;
    all)
        check_files
        build_images
        push_images
        deploy_to_cluster
        ;;
    full)
        check_files
        build_images
        push_images
        deploy_to_cluster
        echo ""
        echo_status "Waiting 30 seconds for services to start..."
        sleep 30
        deploy_test
        echo ""
        watch_test_logs
        ;;
    *)
        echo "Usage: $0 {check|build|push|deploy|test-deploy|test-logs|test-results|info|images|all|full}"
        echo ""
        echo "Commands:"
        echo "  check        - Check if required files exist"
        echo "  build        - Build Docker images (training, prediction, test)"
        echo "  push         - Push images to quay.io"
        echo "  deploy       - Deploy services to CKS cluster"
        echo "  test-deploy  - Deploy test job to cluster"
        echo "  test-logs    - Stream test logs"
        echo "  test-results - Get test results"
        echo "  info         - Show deployment information"
        echo "  images       - List built Docker images"
        echo "  all          - Build, push, and deploy (excluding tests)"
        echo "  full         - Build, push, deploy, and run tests"
        echo ""
        echo "Examples:"
        echo "  $0 build          # Build all images locally"
        echo "  $0 push           # Push to quay.io"
        echo "  $0 deploy         # Deploy to cluster"
        echo "  $0 test-deploy    # Run tests"
        echo "  $0 full           # Complete workflow"
        exit 1
        ;;
esac
