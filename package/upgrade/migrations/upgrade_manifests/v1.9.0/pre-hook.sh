patch_harvester_vm_migration_details_dashboard() {
  echo "Patch configmap/harvester-vm-migration-details-dashboard..."
  local patch_json
  patch_json=$(
    kubectl get -n cattle-dashboards configmap harvester-vm-migration-details-dashboard -o json |
      jq --argjson new_panel '{
        "datasource": {
          "type": "prometheus",
          "uid": "prometheus"
        },
        "description": "Convergence ratio: dirty_rate / transfer_rate. < 1: converging — ≥ 1: not converging, migration will abort. Shows -1 when transfer rate is zero (migration not yet active).",
        "fieldConfig": {
          "defaults": {
            "color": {"mode": "palette-classic"},
            "custom": {
              "axisCenteredZero": false,
              "axisColorMode": "text",
              "axisLabel": "",
              "axisPlacement": "auto",
              "barAlignment": 0,
              "drawStyle": "line",
              "fillOpacity": 10,
              "gradientMode": "none",
              "hideFrom": {"legend": false, "tooltip": false, "viz": false},
              "lineInterpolation": "linear",
              "lineWidth": 1,
              "pointSize": 5,
              "scaleDistribution": {"type": "linear"},
              "showPoints": "auto",
              "spanNulls": false,
              "stacking": {"group": "A", "mode": "none"},
              "thresholdsStyle": {"mode": "line+area"},
              "axisSoftMin": -1.5,
              "axisSoftMax": 2
            },
            "mappings": [],
            "thresholds": {
              "mode": "absolute",
              "steps": [
                {"color": "grey",  "value": null},
                {"color": "green", "value": 0},
                {"color": "red",   "value": 1}
              ]
            },
            "unit": "short"
          },
          "overrides": []
        },
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 16},
        "id": 10,
        "options": {
          "legend": {"calcs": [], "displayMode": "list", "placement": "bottom", "showLegend": false},
          "tooltip": {"mode": "single", "sort": "none"}
        },
        "targets": [
          {
            "datasource": {"type": "prometheus", "uid": "prometheus"},
            "editorMode": "code",
            "expr": "(\n  (\n    kubevirt_vmi_migration_dirty_memory_rate_bytes{namespace=\"$namespace\", name=\"$vm\"}\n    / kubevirt_vmi_migration_memory_transfer_rate_bytes{namespace=\"$namespace\", name=\"$vm\"}\n  )\n  and on(namespace, name) (\n    kubevirt_vmi_migration_memory_transfer_rate_bytes{namespace=\"$namespace\", name=\"$vm\"} > 0\n  )\n)\nor on(namespace, name) (\n  (kubevirt_vmi_migration_memory_transfer_rate_bytes{namespace=\"$namespace\", name=\"$vm\"} == 0)\n  * 0 - 1\n)",
            "legendFormat": "dirty / transfer",
            "range": true,
            "refId": "A"
          }
        ],
        "title": "Migration Memory Dirty vs Transfer Rate Ratio",
        "type": "timeseries"
      }' '
        .data."harvester_vm_migration_details.json" | fromjson |
        .panels |= (
          map(
            if .title == "Migration Memory Transfer Rate" then
              .targets[0].expr = "kubevirt_vmi_migration_memory_transfer_rate_bytes{namespace=\"$namespace\", name=\"$vm\"}"
            else
              .
            end
          ) |
          if map(.title) | index("Migration Memory Dirty vs Transfer Rate Ratio") | not then
            . + [($new_panel | .id = 10)]
          else
            .
          end
        )'
  )
  if [ -z "$patch_json" ]; then
    echo "Something goes wrong, the patch json is empty... skip this patch"
    return 0
  fi
  patch_json=$(echo '{"data":{"harvester_vm_migration_details.json": ""}}' |
    jq --arg arg "$patch_json" '.data."harvester_vm_migration_details.json"=$arg')
  kubectl patch -n cattle-dashboards configmap harvester-vm-migration-details-dashboard \
    --type merge -p "$patch_json"
}
