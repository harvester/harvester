#!/bin/bash -x

patch_harvester_vm_dashboard()
{
    echo "Patch configmap/harvester-vm-dashboard..."
    local patch_json=$( \
        kubectl get -n cattle-dashboards configmap harvester-vm-dashboard -o json | \
            jq '.data."harvester_vm_dashboard.json" | fromjson | .panels |=
                map(
                    if .title == "CPU Usage" then
                        .yaxes |= map(
                            if .format == "percent" then
                                .format = "percentunit"
                            else
                                .
                            end
                        ) |
                        .targets[0].expr = "topk(${count}, (avg(rate(kubevirt_vmi_vcpu_seconds_total[5m])) by (domain, name)) / 1000)"
                    else
                        .
                    end
                )' |
            jq -r .
    )

    if [ -z "$patch_json" ]; then
        echo "Something goes wrong, the patch json is empty... skip this patch"
        return 0
    fi

    patch_json=$(echo '{"data":{"harvester_vm_dashboard.json": ""}}' | jq --arg arg "$patch_json" '.data."harvester_vm_dashboard.json"=$arg')
    kubectl patch -n cattle-dashboards configmap harvester-vm-dashboard --type merge -p "$patch_json"
}

patch_harvester_vm_detail_dashboard()
{
    echo "Patch configmap/harvester-vm-detail-dashboard..."
    local patch_json=$( \
        kubectl get -n cattle-dashboards configmap harvester-vm-detail-dashboard -o json | \
            jq '.data."harvester_vm_info_details.json" | fromjson | .panels |=
                map(
                    if .title == "CPU Usage" then
                        .fieldConfig.defaults.unit = "percentunit" |
                        .targets[0].expr = "sum(avg(rate(kubevirt_vmi_vcpu_seconds_total{namespace=\"$namespace\", name=\"$vm\"}[5m])) by (domain, name)) / 1000"
                    else
                        .
                    end
                )'
    )

    if [ -z "$patch_json" ]; then
        echo "Something goes wrong, the patch json is empty... skip this patch"
        return 0
    fi

    patch_json=$(echo '{"data":{"harvester_vm_info_details.json": ""}}' | jq --arg arg "$patch_json" '.data."harvester_vm_info_details.json"=$arg')
    kubectl patch -n cattle-dashboards configmap harvester-vm-detail-dashboard --type merge -p "$patch_json"
}

apply_harvester_vm_migration_details_dashboard()
{
  echo "Checking if configmap/harvester-vm-migration-details-dashboard exists..."
  if ! kubectl get configmap harvester-vm-migration-details-dashboard -n cattle-dashboards &>/dev/null; then
      echo "ConfigMap not found. Creating configmap/harvester-vm-migration-details-dashboard..."
      kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: cattle-dashboards
  name: harvester-vm-migration-details-dashboard
  labels:
    grafana_dashboard: "1"
data:
  harvester_vm_migration_details.json: |-
    {
      "annotations": {
        "list": [
          {
            "builtIn": 1,
            "datasource": {
              "type": "grafana",
              "uid": "-- Grafana --"
            },
            "enable": true,
            "hide": true,
            "iconColor": "rgba(0, 211, 255, 1)",
            "name": "Annotations & Alerts",
            "target": {
              "limit": 100,
              "matchAny": false,
              "tags": [],
              "type": "dashboard"
            },
            "type": "dashboard"
          }
        ]
      },
      "description": "Harvester VM Migration Details",
      "editable": true,
      "fiscalYearStartMonth": 0,
      "graphTooltip": 0,
      "id": 36,
      "links": [],
      "liveNow": false,
      "panels": [
        {
          "datasource": {
            "type": "prometheus",
            "uid": "prometheus"
          },
          "description": "The remaining guest OS data to be migrated to the new VM.",
          "fieldConfig": {
            "defaults": {
              "color": {
                "mode": "palette-classic"
              },
              "custom": {
                "axisCenteredZero": false,
                "axisColorMode": "text",
                "axisLabel": "",
                "axisPlacement": "auto",
                "barAlignment": 0,
                "drawStyle": "line",
                "fillOpacity": 0,
                "gradientMode": "none",
                "hideFrom": {
                  "legend": false,
                  "tooltip": false,
                  "viz": false
                },
                "lineInterpolation": "linear",
                "lineWidth": 1,
                "pointSize": 5,
                "scaleDistribution": {
                  "type": "linear"
                },
                "showPoints": "auto",
                "spanNulls": false,
                "stacking": {
                  "group": "A",
                  "mode": "none"
                },
                "thresholdsStyle": {
                  "mode": "off"
                }
              },
              "mappings": [],
              "thresholds": {
                "mode": "absolute",
                "steps": [
                  {
                    "color": "green",
                    "value": null
                  },
                  {
                    "color": "red",
                    "value": 80
                  }
                ]
              },
              "unit": "decbytes"
            },
            "overrides": []
          },
          "gridPos": {
            "h": 8,
            "w": 12,
            "x": 0,
            "y": 0
          },
          "id": 2,
          "options": {
            "legend": {
              "calcs": [],
              "displayMode": "list",
              "placement": "bottom",
              "showLegend": false
            },
            "timezone": [
              ""
            ],
            "tooltip": {
              "mode": "single",
              "sort": "none"
            }
          },
          "targets": [
            {
              "datasource": {
                "type": "prometheus",
                "uid": "prometheus"
              },
              "editorMode": "code",
              "exemplar": false,
              "expr": "kubevirt_vmi_migration_data_remaining_bytes{namespace=\"\$namespace\", name=\"\$vm\"}",
              "hide": false,
              "legendFormat": "remaining bytes",
              "range": true,
              "refId": "A"
            }
          ],
          "title": "Migration Data Remaining Bytes",
          "type": "timeseries"
        },
        {
          "datasource": {
            "type": "prometheus",
            "uid": "prometheus"
          },
          "description": "The total Guest OS data processed and migrated to the new VM.",
          "fieldConfig": {
            "defaults": {
              "color": {
                "mode": "palette-classic"
              },
              "custom": {
                "axisCenteredZero": false,
                "axisColorMode": "text",
                "axisLabel": "",
                "axisPlacement": "auto",
                "barAlignment": 0,
                "drawStyle": "line",
                "fillOpacity": 0,
                "gradientMode": "none",
                "hideFrom": {
                  "legend": false,
                  "tooltip": false,
                  "viz": false
                },
                "lineInterpolation": "linear",
                "lineWidth": 1,
                "pointSize": 5,
                "scaleDistribution": {
                  "type": "linear"
                },
                "showPoints": "auto",
                "spanNulls": false,
                "stacking": {
                  "group": "A",
                  "mode": "none"
                },
                "thresholdsStyle": {
                  "mode": "off"
                }
              },
              "mappings": [],
              "thresholds": {
                "mode": "absolute",
                "steps": [
                  {
                    "color": "green",
                    "value": null
                  },
                  {
                    "color": "red",
                    "value": 80
                  }
                ]
              },
              "unit": "decbytes"
            },
            "overrides": []
          },
          "gridPos": {
            "h": 8,
            "w": 12,
            "x": 12,
            "y": 0
          },
          "id": 4,
          "options": {
            "legend": {
              "calcs": [],
              "displayMode": "list",
              "placement": "bottom",
              "showLegend": false
            },
            "tooltip": {
              "mode": "single",
              "sort": "none"
            }
          },
          "targets": [
            {
              "datasource": {
                "type": "prometheus",
                "uid": "prometheus"
              },
              "editorMode": "code",
              "expr": "kubevirt_vmi_migration_data_processed_bytes{namespace=\"\$namespace\", name=\"\$vm\"}",
              "legendFormat": "processed bytes",
              "range": true,
              "refId": "A"
            }
          ],
          "title": "Migration Data Processed Bytes",
          "type": "timeseries"
        },
        {
          "datasource": {
            "type": "prometheus",
            "uid": "prometheus"
          },
          "description": "The rate at which the memory is being transferred.",
          "fieldConfig": {
            "defaults": {
              "color": {
                "mode": "palette-classic"
              },
              "custom": {
                "axisCenteredZero": false,
                "axisColorMode": "text",
                "axisLabel": "",
                "axisPlacement": "auto",
                "barAlignment": 0,
                "drawStyle": "line",
                "fillOpacity": 0,
                "gradientMode": "none",
                "hideFrom": {
                  "legend": false,
                  "tooltip": false,
                  "viz": false
                },
                "lineInterpolation": "linear",
                "lineWidth": 1,
                "pointSize": 5,
                "scaleDistribution": {
                  "type": "linear"
                },
                "showPoints": "auto",
                "spanNulls": false,
                "stacking": {
                  "group": "A",
                  "mode": "none"
                },
                "thresholdsStyle": {
                  "mode": "off"
                }
              },
              "mappings": [],
              "thresholds": {
                "mode": "absolute",
                "steps": [
                  {
                    "color": "green",
                    "value": null
                  },
                  {
                    "color": "red",
                    "value": 80
                  }
                ]
              },
              "unit": "Bps"
            },
            "overrides": []
          },
          "gridPos": {
            "h": 8,
            "w": 12,
            "x": 0,
            "y": 8
          },
          "id": 8,
          "options": {
            "legend": {
              "calcs": [],
              "displayMode": "list",
              "placement": "bottom",
              "showLegend": false
            },
            "tooltip": {
              "mode": "single",
              "sort": "none"
            }
          },
          "targets": [
            {
              "datasource": {
                "type": "prometheus",
                "uid": "prometheus"
              },
              "editorMode": "code",
              "expr": "kubevirt_vmi_migration_disk_transfer_rate_bytes{namespace=\"\$namespace\", name=\"\$vm\"}",
              "legendFormat": "memory transfer rate",
              "range": true,
              "refId": "A"
            }
          ],
          "title": "Migration Memory Transfer Rate",
          "type": "timeseries"
        },
        {
          "datasource": {
            "type": "prometheus",
            "uid": "prometheus"
          },
          "description": "The rate of memory being dirty in the Guest OS.",
          "fieldConfig": {
            "defaults": {
              "color": {
                "mode": "palette-classic"
              },
              "custom": {
                "axisCenteredZero": false,
                "axisColorMode": "text",
                "axisLabel": "",
                "axisPlacement": "auto",
                "barAlignment": 0,
                "drawStyle": "line",
                "fillOpacity": 0,
                "gradientMode": "none",
                "hideFrom": {
                  "legend": false,
                  "tooltip": false,
                  "viz": false
                },
                "lineInterpolation": "linear",
                "lineWidth": 1,
                "pointSize": 5,
                "scaleDistribution": {
                  "type": "linear"
                },
                "showPoints": "auto",
                "spanNulls": false,
                "stacking": {
                  "group": "A",
                  "mode": "none"
                },
                "thresholdsStyle": {
                  "mode": "off"
                }
              },
              "mappings": [],
              "thresholds": {
                "mode": "absolute",
                "steps": [
                  {
                    "color": "green",
                    "value": null
                  },
                  {
                    "color": "red",
                    "value": 80
                  }
                ]
              },
              "unit": "Bps"
            },
            "overrides": []
          },
          "gridPos": {
            "h": 8,
            "w": 12,
            "x": 12,
            "y": 8
          },
          "id": 6,
          "options": {
            "legend": {
              "calcs": [],
              "displayMode": "list",
              "placement": "bottom",
              "showLegend": false
            },
            "tooltip": {
              "mode": "single",
              "sort": "none"
            }
          },
          "targets": [
            {
              "datasource": {
                "type": "prometheus",
                "uid": "prometheus"
              },
              "editorMode": "code",
              "expr": "kubevirt_vmi_migration_dirty_memory_rate_bytes{namespace=\"\$namespace\", name=\"\$vm\"}",
              "legendFormat": "dirty memory rate",
              "range": true,
              "refId": "A"
            }
          ],
          "title": "Migration Dirty Memory Rate",
          "type": "timeseries"
        }
      ],
      "schemaVersion": 37,
      "style": "dark",
      "tags": [],
      "templating": {
        "list": [
          {
            "current": {
              "selected": false,
              "text": "default",
              "value": "default"
            },
            "datasource": {
              "type": "prometheus",
              "uid": "prometheus"
            },
            "definition": "label_values(kubevirt_vmi_migration_data_remaining_bytes, namespace)",
            "hide": 0,
            "includeAll": false,
            "label": "namespace",
            "multi": false,
            "name": "namespace",
            "options": [],
            "query": {
              "query": "label_values(kubevirt_vmi_migration_data_remaining_bytes, namespace)",
              "refId": "StandardVariableQuery"
            },
            "refresh": 1,
            "regex": "",
            "skipUrlSync": false,
            "sort": 0,
            "type": "query"
          },
          {
            "current": {
              "selected": true,
              "text": "test",
              "value": "test"
            },
            "datasource": {
              "type": "prometheus",
              "uid": "prometheus"
            },
            "definition": "label_values(kubevirt_vmi_migration_data_remaining_bytes, name)",
            "hide": 0,
            "includeAll": false,
            "label": "vm",
            "multi": false,
            "name": "vm",
            "options": [],
            "query": {
              "query": "label_values(kubevirt_vmi_migration_data_remaining_bytes, name)",
              "refId": "StandardVariableQuery"
            },
            "refresh": 1,
            "regex": "",
            "skipUrlSync": false,
            "sort": 0,
            "type": "query"
          }
        ]
      },
      "time": {
        "from": "now-3h",
        "to": "now"
      },
      "timepicker": {},
      "timezone": "",
      "title": "Harvester VM Migration Details",
      "uid": "harvester-vm-migration-details-1",
      "version": 1,
      "weekStart": ""
    }
EOF
    else
        echo "ConfigMap already exists. Skipping creation."
    fi
}

# Fix incorrect unit in CPU Usage dashboard https://github.com/harvester/harvester/issues/7086
patch_harvester_vm_dashboard
patch_harvester_vm_detail_dashboard
apply_harvester_vm_migration_details_dashboard
