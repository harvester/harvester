import requests
import os
import sys
import json
from datetime import datetime, timedelta


GITHUB_GRAPHQL_URL = "https://api.github.com/graphql"

# Add emoji reactions to Slack message for better visibility
def add_emoji_reactions(channel_id, message_ts, headers):
    """Add emoji reactions to a Slack message"""
    reactions_api_url = "https://slack.com/api/reactions.add"
    emojis = ["white_check_mark", "+1", "pray"]

    for emoji in emojis:
        reaction_payload = {
            "channel": channel_id,
            "timestamp": message_ts,
            "name": emoji
        }

        try:
            reaction_response = requests.post(reactions_api_url,
                                              json=reaction_payload,
                                              headers=headers)
            if reaction_response.status_code == 200:
                reaction_data = reaction_response.json()
                if not reaction_data.get('ok'):
                    print(f"Failed to add {emoji} reaction: " +
                          f"{reaction_data.get('error')}")
                else:
                    print(f"✅ Added {emoji} reaction successfully")
            else:
                print(f"HTTP error adding {emoji} reaction: " +
                      f"{reaction_response.status_code}")
        except Exception as e:
            print(f"Error adding {emoji} reaction: {e}")


def get_github_project_info(github_token, github_org, github_project):
    headers = {
        "Authorization": f"Bearer {github_token}",
        "Content-Type": "application/json"
    }
    query = '''
    {
      organization(login: "%s") {
        projectsV2(first: 20) {
          nodes {
            id
            title
            number
          }
        }
      }
    }
    ''' % (github_org)
    payload = {
        "query": query
    }

    response = requests.post(GITHUB_GRAPHQL_URL, headers=headers, json=payload)
    if response.status_code == 200:
        # Find project by title
        print("Response: %s" % response.json())
        nodes = response.json().get("data").get("organization").get("projectsV2").get("nodes")
        for node in nodes:
            if node.get("title") == github_project:
                return node
    else:
        response.raise_for_status()


def get_current_sprint(github_token, project_id):
    headers = {
        "Authorization": f"Bearer {github_token}",
        "Content-Type": "application/json"
    }
    query = '''
    query {
      node(id: "%s") {
        ... on ProjectV2 {
          fields(first: 20) {
            nodes {
              ... on ProjectV2IterationField {
                configuration {
                  iterations {
                    startDate
                    id
                  }
                }
              }
            }
          }
        }
      }
    }
    ''' % (project_id)

    payload = {
        "query": query
    }

    response = requests.post(GITHUB_GRAPHQL_URL, headers=headers, json=payload)
    if response.status_code == 200:
        # Find project by title
        result = response.json().get("data").get("node").get("fields").get("nodes")
        filtered_result = [node for node in result if 'configuration' in node]
        iterations = filtered_result[0].get("configuration").get("iterations")

        # Find current iteration
        current_date = datetime.now().date()
        current_iteration = None
        for iteration in iterations:
            start_date = datetime.strptime(iteration['startDate'], "%Y-%m-%d").date()
            end_date = start_date + timedelta(days=13)
            if start_date <= current_date <= end_date:
                current_iteration = iteration
                break

        return current_iteration
    else:
        response.raise_for_status()


def is_today_is_in_last_day_of_current_sprint(github_token, project_id):
    current_iteration = get_current_sprint(github_token, project_id)
    if current_iteration is None:
        print("Current sprint not found")
        return False

    current_date = datetime.now().date()
    end_date = datetime.strptime(current_iteration['startDate'], "%Y-%m-%d").date() + timedelta(days=13)

    return current_date == end_date


def list_issues_in_project(github_token, project_id, desired_status=None):
    headers = {
        "Authorization": f"Bearer {github_token}",
        "Content-Type": "application/json"
    }

    query = """
    query($project: ID!, $cursor: String) {
      node(id: $project) {
        ... on ProjectV2 {
          items(first: 100, after: $cursor) {
            nodes {
              content {
                ... on Issue {
                  number
                  title
                  assignees(first: 10) {
                    nodes {
                      login
                    }
                  }
                }
              }
              status: fieldValueByName(name: "Status") {
                ... on ProjectV2ItemFieldSingleSelectValue {
                  name
                }
              }
              sprint: fieldValueByName(name: "Sprint") {
                ... on ProjectV2ItemFieldIterationValue {
                  title
                  startDate
                }
              }
            }
            pageInfo {
              endCursor
              hasNextPage
            }
          }
        }
      }
    }
    """

    cursor = None

    current_issues = []
    non_current_issues = []

    current_sprint = get_current_sprint(github_token, project_id)
    print(f"Current sprint: {current_sprint}")

    while True:
        variables = {"project": project_id, "cursor": cursor}
        response = requests.post(GITHUB_GRAPHQL_URL,
                                 headers=headers,
                                 json={"query": query, "variables": variables})

        if response.status_code == 200:
            data = response.json()
            items = data['data']['node']['items']['nodes']
            for item in items:
                if item['status'] is None:
                    # example
                    # {'content': {}, 'status': None, 'sprint': None}
                    continue
                status = item['status']['name']
                if desired_status and status not in desired_status:
                    continue
                sprint = item['sprint']
                if not sprint or not sprint.get('startDate') or not current_sprint or sprint['startDate'] != current_sprint['startDate']:
                    non_current_issues.append(item)
                else:
                    current_issues.append(item)

            page_info = data['data']['node']['items']['pageInfo']
            if page_info['hasNextPage']:
                cursor = page_info['endCursor']
            else:
                break
        else:
            raise Exception(f"Query failed to run by returning code of {response.status_code}. {response.text}")

    return current_issues, non_current_issues


def flatten_issues(title, blocks, issues, user_mapping, issue_template_url):
    # Append the title and divider only if there are issues to display
    if issues:
        blocks.append(
            {
                "type": "section",
                "text": {
                    "type": "mrkdwn",
                    "text": f"*{title}* - {len(issues)} issues"
                }
            }
        )
        blocks.append({"type": "divider"})

        # Combine issues into chunks of 5
        issue_texts = []
        for i, issue in enumerate(issues):
            if issue["content"] == {}:
                # example
                # {'content': {}, 'status': {'name': 'Ready For Testing'}, 'sprint': {'title': 'Sprint 18', 'startDate': '2025-07-07'}}
                continue
            number = issue["content"]["number"]
            title = issue["content"]["title"]
            issue_url = f"{issue_template_url}{number}"
            assignees = []
            for assignee in issue["content"]["assignees"]["nodes"]:
                slack_id = user_mapping.get(assignee["login"])
                if not slack_id:
                    assignees.append(assignee["login"])
                else:
                    assignees.append(f"<@{slack_id}>")

            issue_texts.append(f"- *<{issue_url}|{number}>* - {title} - {', '.join(assignees)}")

            # Add a block for every 2 issues for avoiding bad request error
            if (i + 1) % 2 == 0 or (i + 1) == len(issues):
                blocks.append({
                    "type": "section",
                    "text": {
                        "type": "mrkdwn",
                        "text": "\n".join(issue_texts)  # Combine all issue texts
                    }
                })
                issue_texts = []  # Reset for the next chunk

    return blocks


def send_slack_notification(user_mapping, current_issues, non_current_issues,
                            group_id, group_name, issue_template_url,
                            slack_bot_token, slack_channel):
    if len(current_issues) == 0 and len(non_current_issues) == 0:
        print("Nothing to notify")
        return

    # Send summary message first using chat.postMessage API
    total_issues = len(current_issues) + len(non_current_issues)
    summary_blocks = [{
        "type": "section",
        "text": {
            "type": "mrkdwn",
            "text": f"Hello <!subteam^{group_id}|{group_name}>, " +
                    "this is a reminder. \n\n" +
                    f"There are *{total_issues}* 'Ready for Testing' " +
                    "or 'Testing' issues:\n" +
                    f"  - {len(current_issues)} from previous sprint\n" +
                    f"  - {len(non_current_issues)} from older sprints\n\n" +
                    "Please finish verifying them using the corresponding " +
                    "sprint release soon. Details in thread 👇"
        }
    }]

    summary_payload = {
        "channel": slack_channel,
        "blocks": summary_blocks
    }

    headers = {
        'Content-Type': 'application/json',
        'Authorization': f'Bearer {slack_bot_token}'
    }

    # Send summary message using Slack API
    slack_api_url = "https://slack.com/api/chat.postMessage"
    response = requests.post(slack_api_url, json=summary_payload,
                             headers=headers)
    response.raise_for_status()

    # Get the timestamp of the summary message to reply in thread
    response_data = response.json()

    if not response_data.get('ok'):
        print(f"Slack API error: {response_data.get('error')}")
        return

    thread_ts = response_data.get('ts')

    if thread_ts:
        # Send instructions message in thread first
        instruction_blocks = [{
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": "*Instructions:*\n" +
                        "  - If passed, move them to 'Closed' and " +
                        "DO NOT change the sprint. \n" +
                        "  - If not passed, move them to 'Implementation' " +
                        "and update the sprint to the current one. \n\n" +
                        "Thanks for your efforts!"
            }
        }]

        instruction_payload = {
            "channel": slack_channel,
            "blocks": instruction_blocks,
            "thread_ts": thread_ts
        }

        # Send instructions message
        response = requests.post(slack_api_url, json=instruction_payload,
                                 headers=headers)
        response.raise_for_status()

        instruction_response_data = response.json()
        if instruction_response_data.get('ok'):
            instruction_ts = instruction_response_data.get('ts')
            instruction_channel_id = instruction_response_data.get('channel')
            add_emoji_reactions(instruction_channel_id,
                                instruction_ts, headers)
        else:
            print("Slack API error for instruction thread message: " +
                  f"{instruction_response_data.get('error')}")

        # Send previous sprint issues in separate thread message
        if current_issues:
            previous_sprint_blocks = []
            previous_sprint_blocks.append({"type": "divider"})
            previous_sprint_blocks = flatten_issues(
                "Ready for Testing Issues from Previous Sprint",
                previous_sprint_blocks, current_issues, user_mapping,
                issue_template_url)

            previous_sprint_payload = {
                "channel": slack_channel,
                "blocks": previous_sprint_blocks,
                "thread_ts": thread_ts
            }

            response = requests.post(slack_api_url,
                                     json=previous_sprint_payload,
                                     headers=headers)
            response.raise_for_status()

            previous_sprint_response_data = response.json()
            if previous_sprint_response_data.get('ok'):
                previous_sprint_ts = previous_sprint_response_data.get('ts')
                prev_channel_id = previous_sprint_response_data.get('channel')
                add_emoji_reactions(prev_channel_id,
                                    previous_sprint_ts, headers)
            else:
                print("Slack API error for previous sprint thread message: " +
                      f"{previous_sprint_response_data.get('error')}")

        # Send older sprints issues in separate thread message
        if non_current_issues:
            older_sprints_blocks = []
            older_sprints_blocks.append({"type": "divider"})
            older_sprints_blocks = flatten_issues(
                "Ready for Testing Issues from Older Sprints",
                older_sprints_blocks, non_current_issues, user_mapping,
                issue_template_url)

            older_sprints_payload = {
                "channel": slack_channel,
                "blocks": older_sprints_blocks,
                "thread_ts": thread_ts
            }

            response = requests.post(slack_api_url,
                                     json=older_sprints_payload,
                                     headers=headers)
            response.raise_for_status()

            older_sprints_response_data = response.json()
            if older_sprints_response_data.get('ok'):
                older_sprints_ts = older_sprints_response_data.get('ts')
                older_channel_id = older_sprints_response_data.get('channel')
                add_emoji_reactions(older_channel_id,
                                    older_sprints_ts, headers)
            else:
                print("Slack API error for older sprints thread message: " +
                      f"{older_sprints_response_data.get('error')}")


def scan_and_notify(github_org, github_repo, github_project):
    github_token = os.getenv("GITHUB_TOKEN")
    value = os.getenv("USER_MAPPING")
    slack_bot_token = os.getenv("SLACK_BOT_TOKEN")
    slack_channel = os.getenv("SLACK_CHANNEL")
    issue_template_url = os.getenv("ISSUE_TEMPLATE_URL")
    group_id = os.getenv("GROUP_ID")
    group_name = os.getenv("GROUP_NAME")

    if not slack_bot_token or not slack_channel:
        print("SLACK_BOT_TOKEN and SLACK_CHANNEL must be provided " +
              "for thread functionality")
        return

    user_mapping = {}
    if value is not None:
        # Example: {"{github_username}": "{slack_user_id}"}
        # os.getenv("USER_MAPPING") is a json string
        user_mapping = json.loads(value)

    project = get_github_project_info(github_token, github_org, github_project)
    print(f"GitHub Project Details: {project}")

    last_day = is_today_is_in_last_day_of_current_sprint(
        github_token, project.get("id"))
    if not last_day:
        print("Today %s is not in last day of current sprint" %
              datetime.now().date())
        return

    project_id = project.get("id")
    current_issues, non_current_issues = list_issues_in_project(
        github_token,
        project_id,
        ["Ready For Testing", "Testing"])

    print("Number of \"Ready For Testing\" and \"Testing\" issues " +
          "for current sprint:", len(current_issues))
    print("Number of \"Ready For Testing\" and \"Testing\" issues " +
          "for non-current sprint:", len(non_current_issues))

    send_slack_notification(user_mapping, current_issues, non_current_issues,
                            group_id, group_name, issue_template_url,
                            slack_bot_token, slack_channel)


if __name__ == "__main__":
    if len(sys.argv) < 4:
        print('Usage: python scan-and-notify-testing-items.py ' +
              '<github_org> <github_repo> <github_project>')
        sys.exit(1)

    scan_and_notify(sys.argv[1], sys.argv[2], sys.argv[3])
