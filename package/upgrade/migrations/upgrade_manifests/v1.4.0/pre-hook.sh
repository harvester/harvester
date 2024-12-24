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
                            if .format == "percentunit" then
                                .format = "percent"
                            else
                                .
                            end
                    )
                    else
                        .
                    end
                )' |
        jq -r .
    )

    if [ -z "$patch_json" ]; then
        echo "Something goes wrong, the patch json is empty... skip this patch"
        return 1
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
                        .fieldConfig.defaults.unit = "percent"
                    else
                        .
                    end
                )'
    )

    if [ -z "$patch_json" ]; then
        echo "Something goes wrong, the patch json is empty... skip this patch"
        return 1
    fi

    patch_json=$(echo '{"data":{"harvester_vm_info_details.json": ""}}' | jq --arg arg "$patch_json" '.data."harvester_vm_info_details.json"=$arg')
    kubectl patch -n cattle-dashboards configmap harvester-vm-detail-dashboard --type merge -p "$patch_json"
}

# Fix incorrect unit in CPU Usage dashboard https://github.com/harvester/harvester/issues/7086
patch_harvester_vm_dashboard
patch_harvester_vm_detail_dashboard
