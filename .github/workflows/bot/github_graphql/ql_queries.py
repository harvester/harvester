GET_ISSUE_QUERY = """
query($repo_owner: String!, $repo_name: String!, $issue_number: Int!) {
  repository(owner: $repo_owner, name: $repo_name) {
    issue(number: $issue_number) {
      id
      number
      title
      url
    }
  }
}
"""


GET_ORGANIZATION_PROJECT_QUERY = """
query($organization: String!, $project_number: Int!) {
  organization(login: $organization) {
    projectV2(number: $project_number) {
      id
      title
      number
      fields(first: 20) {
        nodes {
          ... on ProjectV2FieldCommon {
            id
            name
          }
          ... on ProjectV2SingleSelectField {
            id
            name
            options {
              id
              name
            }
          }
        }
      }
    }
  }
}
"""

# This is used to test
GET_USER_PROJECT_QUERY = """
query($organization: String!, $project_number: Int!) {
  user(login: $organization) {
    projectV2(number: $project_number) {
      id
      title
      number
    }
  }
}
"""

GET_GLOBAL_ISSUE_QUERY = """
query Organization($issue_node_id: ID!) {
    node(id: $issue_node_id) {
        ... on Issue {
            title
            id
            number
        }
    }
}
"""