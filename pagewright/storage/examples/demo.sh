#!/bin/bash

# Example usage script for PageWright Storage Service
# This demonstrates all the available API endpoints

set -e

BASE_URL="${PAGEWRIGHT_STORAGE_URL:-http://localhost:8080}"
SITE_ID="demo-site-$(date +%s)"
BUILD_ID="build-001"

echo "PageWright Storage Service - API Examples"
echo "=========================================="
echo ""
echo "Using Base URL: $BASE_URL"
echo "Site ID: $SITE_ID"
echo "Build ID: $BUILD_ID"
echo ""

# Health check
echo "1. Health Check"
echo "   GET $BASE_URL/health"
curl -s "$BASE_URL/health" | jq .
echo ""

# Create a test artifact
echo "2. Create Test Artifact"
echo "   Creating test.tar.gz with sample content..."
mkdir -p /tmp/demo-site
echo "Sample content for demo site" > /tmp/demo-site/index.html
tar -czf /tmp/demo-artifact.tar.gz -C /tmp demo-site
rm -rf /tmp/demo-site
echo "   Artifact created: /tmp/demo-artifact.tar.gz"
echo ""

# Upload artifact
echo "3. Upload Artifact"
echo "   PUT $BASE_URL/sites/$SITE_ID/artifacts/$BUILD_ID"
curl -s -X PUT \
  "$BASE_URL/sites/$SITE_ID/artifacts/$BUILD_ID" \
  --data-binary @/tmp/demo-artifact.tar.gz | jq .
echo ""

# Write log entry
echo "4. Write Log Entry"
echo "   POST $BASE_URL/sites/$SITE_ID/logs"
curl -s -X POST \
  "$BASE_URL/sites/$SITE_ID/logs" \
  -H "Content-Type: application/json" \
  -d '{
    "build_id": "'"$BUILD_ID"'",
    "action": "build",
    "status": "success",
    "metadata": {
      "user": "demo-user",
      "branch": "main",
      "commit": "abc123"
    }
  }' | jq .
echo ""

# Create second build
BUILD_ID_2="build-002"
echo "5. Upload Second Artifact"
echo "   PUT $BASE_URL/sites/$SITE_ID/artifacts/$BUILD_ID_2"
curl -s -X PUT \
  "$BASE_URL/sites/$SITE_ID/artifacts/$BUILD_ID_2" \
  --data-binary @/tmp/demo-artifact.tar.gz | jq .
echo ""

echo "6. Write Second Log Entry"
echo "   POST $BASE_URL/sites/$SITE_ID/logs"
curl -s -X POST \
  "$BASE_URL/sites/$SITE_ID/logs" \
  -H "Content-Type: application/json" \
  -d '{
    "build_id": "'"$BUILD_ID_2"'",
    "action": "deploy",
    "status": "success",
    "metadata": {
      "user": "demo-user",
      "environment": "production"
    }
  }' | jq .
echo ""

# List versions
echo "7. List All Versions"
echo "   GET $BASE_URL/sites/$SITE_ID/versions"
curl -s "$BASE_URL/sites/$SITE_ID/versions" | jq .
echo ""

# Download artifact
echo "8. Download Artifact"
echo "   GET $BASE_URL/sites/$SITE_ID/artifacts/$BUILD_ID"
curl -s "$BASE_URL/sites/$SITE_ID/artifacts/$BUILD_ID" \
  -o /tmp/downloaded-artifact.tar.gz
echo "   Downloaded to: /tmp/downloaded-artifact.tar.gz"
echo "   Size: $(ls -lh /tmp/downloaded-artifact.tar.gz | awk '{print $5}')"
echo ""

# Cleanup
echo "9. Cleanup"
rm -f /tmp/demo-artifact.tar.gz /tmp/downloaded-artifact.tar.gz
echo "   Temporary files cleaned up"
echo ""

echo "=========================================="
echo "Demo completed successfully!"
echo ""
echo "To view the stored data, check the NFS volume:"
echo "  docker exec pagewright-storage ls -la /nfs/sites/$SITE_ID/"
