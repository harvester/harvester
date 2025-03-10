import requests
from bot.github_graphql.ql_queries import GET_ISSUE_QUERY, GET_GLOBAL_ISSUE_QUERY, GET_ORGANIZATION_PROJECT_QUERY
from bot.github_graphql.ql_mutation import ADD_ISSUE_TO_PROJECT_MUTATION, MOVE_ISSUE_TO_STATUS

class GitHubProjectManager:
    def __init__(self, organization, repository, project_number, headers):
        self.organization = organization
        self.repository = repository
        self.headers = headers
        self.url = "https://api.github.com/graphql"
        self.__project = self.__get_orgnization_project(project_number)
        self.status_node_id, self.status = self.get_status_fields()
            
    def get_status_fields(self):
            nodes = self.__project.get("fields").get("nodes")
            for node in nodes:
                if node.get("name") == "Status":
                    return node.get("id"), {option.get("name"): option.get("id") for option in node.get("options")}

    def project(self):
        return self.__project

    def get_issue(self, issue_number):
        variables = {
            'repo_owner': self.organization,
            'repo_name': self.repository,
            'issue_number': issue_number
        }
        response = requests.post(self.url, headers=self.headers, json={'query': GET_ISSUE_QUERY, 'variables': variables})
        if response.status_code == 200:
            return response.json()['data']['repository']['issue']
        else:
            raise Exception(f"Query failed to run by returning code of {response.status_code}. {response.json()}")
        
    def get_global_issue(self, issue_node_id):
        variables = {
            'issue_node_id': issue_node_id
        }
        response = requests.post(self.url, headers=self.headers, json={'query': GET_GLOBAL_ISSUE_QUERY, 'variables': variables})
        if response.status_code == 200:
            return response.json()['data']['node']
        else:
            raise Exception(f"Query failed to run by returning code of {response.status_code}. {response.json})")

    def add_issue_to_project(self, issue_id):
        variables = {
            'project_id': self.__project["id"],
            'content_id': issue_id
        }
        response = requests.post(self.url, headers=self.headers, json={'query': ADD_ISSUE_TO_PROJECT_MUTATION, 'variables': variables})
        if response.status_code == 200:
            return response.json()
        else:
            raise Exception(f"Mutation failed to run by returning code of {response.status_code}. {response.json()}")
        
    def move_issue_to_status(self, issue_id, status_name):
        variables = {
            'project_id': self.__project["id"],
            'item_id': issue_id,
            'field_id': self.status_node_id,
            'single_select_option_id': self.status[status_name]
        }
        
        response = requests.post(self.url, headers=self.headers, json={'query': MOVE_ISSUE_TO_STATUS, 'variables': variables})
        if response.status_code == 200:
            return response.json()
        else:
            raise Exception(f"Mutation failed to run by returning code of {response.status_code}. {response.json()}")
    
    def __get_orgnization_project(self, project_number):
        variables = {
            'organization': self.organization,
            'project_number': project_number
        }
        response = requests.post(self.url, headers=self.headers, json={'query': GET_ORGANIZATION_PROJECT_QUERY, 'variables': variables})
        if response.status_code == 200:
            return response.json()['data']['organization']['projectV2']
        else:
            raise Exception(f"Query failed to run by returning code of {response.status_code}. {response.json()}")