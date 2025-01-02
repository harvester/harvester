ADD_ISSUE_TO_PROJECT_MUTATION = """
mutation($project_id: ID!, $content_id: ID!) {
  addProjectV2ItemById(input: {projectId: $project_id, contentId: $content_id}) {
    item {
      id
    }
  }
}
"""

MOVE_ISSUE_TO_STATUS = """
  mutation($project_id: ID!, $item_id: ID!, $field_id: ID!, $single_select_option_id: String!) {
    updateProjectV2ItemFieldValue(input: {projectId: $project_id, itemId: $item_id, fieldId: $field_id, value: {singleSelectOptionId: $single_select_option_id}}) {
      projectV2Item {
        id
      }
    }
  }
"""