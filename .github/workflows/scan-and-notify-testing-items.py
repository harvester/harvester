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

    # Separate issues by status and sprint
    current_ready_for_testing = []
    non_current_ready_for_testing = []
    current_testing = []
    non_current_testing = []

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
                is_current_sprint = sprint and sprint.get('startDate') and current_sprint and sprint['startDate'] == current_sprint['startDate']
                
                # Categorize by status and sprint
                if status == "Ready For Testing":
                    if is_current_sprint:
                        current_ready_for_testing.append(item)
                    else:
                        non_current_ready_for_testing.append(item)
                elif status == "Testing":
                    if is_current_sprint:
                        current_testing.append(item)
                    else:
                        non_current_testing.append(item)

            page_info = data['data']['node']['items']['pageInfo']
            if page_info['hasNextPage']:
                cursor = page_info['endCursor']
            else:
                break
        else:
            raise Exception(f"Query failed to run by returning code of {response.status_code}. {response.text}")

    return {
        'current_ready_for_testing': current_ready_for_testing,
        'non_current_ready_for_testing': non_current_ready_for_testing,
        'current_testing': current_testing,
        'non_current_testing': non_current_testing
    }


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


def send_thread_message(slack_api_url, headers, channel, blocks, thread_ts,
                       message_type):
    """Send a thread message with blocks and add emoji reactions"""
    payload = {
        "channel": channel,
        "blocks": blocks,
        "thread_ts": thread_ts,
        "unfurl_links": False,
        "unfurl_media": False
    }

    response = requests.post(slack_api_url, json=payload, headers=headers)
    response.raise_for_status()

    response_data = response.json()
    if response_data.get('ok'):
        message_ts = response_data.get('ts')
        channel_id = response_data.get('channel')
        add_emoji_reactions(channel_id, message_ts, headers)
    else:
        print(f"Slack API error for {message_type} thread message: " +
              f"{response_data.get('error')}")


def send_slack_notification(user_mapping, issues_dict,
                            group_id, group_name, issue_template_url,
                            slack_bot_token, slack_channel):
    current_ready = issues_dict['current_ready_for_testing']
    non_current_ready = issues_dict['non_current_ready_for_testing']
    current_testing = issues_dict['current_testing']
    non_current_testing = issues_dict['non_current_testing']
    
    total_ready = len(current_ready) + len(non_current_ready)
    total_testing = len(current_testing) + len(non_current_testing)
    
    if total_ready == 0 and total_testing == 0:
        print("Nothing to notify")
        return

    # Build summary message with separate counts
    summary_text = f"Hello <!subteam^{group_id}|{group_name}>, this is a reminder. \n\n"
    
    if total_ready > 0:
        summary_text += f"There are *{total_ready}* 'Ready for Testing' issues:\n"
        summary_text += f"  - {len(current_ready)} from previous sprint\n"
        summary_text += f"  - {len(non_current_ready)} from older sprints\n"
    
    if total_testing > 0:
        if total_ready > 0:
            summary_text += "\n"
        summary_text += f"There are *{total_testing}* 'Testing' issues:\n"
        summary_text += f"  - {len(current_testing)} from previous sprint\n"
        summary_text += f"  - {len(non_current_testing)} from older sprints\n"
    
    summary_text += "\nPlease finish verifying them using the corresponding sprint release soon. Details in thread 👇"
    
    summary_blocks = [{
        "type": "section",
        "text": {
            "type": "mrkdwn",
            "text": summary_text
        }
    }]

    summary_payload = {
        "channel": slack_channel,
        "blocks": summary_blocks,
        "unfurl_links": False,
        "unfurl_media": False
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
            "thread_ts": thread_ts,
            "unfurl_links": False,
            "unfurl_media": False
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

        # Send previous sprint Ready for Testing issues
        if current_ready:
            previous_sprint_blocks = []
            previous_sprint_blocks.append({"type": "divider"})
            previous_sprint_blocks = flatten_issues(
                "Ready for Testing Issues from Previous Sprint",
                previous_sprint_blocks, current_ready, user_mapping,
                issue_template_url)

            send_thread_message(slack_api_url, headers, slack_channel,
                              previous_sprint_blocks, thread_ts,
                              "previous sprint Ready for Testing")

        # Send older sprints Ready for Testing issues
        if non_current_ready:
            older_sprints_blocks = []
            older_sprints_blocks.append({"type": "divider"})
            older_sprints_blocks = flatten_issues(
                "Ready for Testing Issues from Older Sprints",
                older_sprints_blocks, non_current_ready, user_mapping,
                issue_template_url)

            send_thread_message(slack_api_url, headers, slack_channel,
                              older_sprints_blocks, thread_ts,
                              "older sprints Ready for Testing")

        # Send previous sprint Testing issues
        if current_testing:
            testing_blocks = []
            testing_blocks.append({"type": "divider"})
            testing_blocks = flatten_issues(
                "Testing Issues from Previous Sprint",
                testing_blocks, current_testing, user_mapping,
                issue_template_url)

            send_thread_message(slack_api_url, headers, slack_channel,
                              testing_blocks, thread_ts,
                              "previous sprint Testing")

        # Send older sprints Testing issues
        if non_current_testing:
            older_testing_blocks = []
            older_testing_blocks.append({"type": "divider"})
            older_testing_blocks = flatten_issues(
                "Testing Issues from Older Sprints",
                older_testing_blocks, non_current_testing, user_mapping,
                issue_template_url)

            send_thread_message(slack_api_url, headers, slack_channel,
                              older_testing_blocks, thread_ts,
                              "older sprints Testing")


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
    issues_dict = list_issues_in_project(
        github_token,
        project_id,
        ["Ready For Testing", "Testing"])

    current_ready = issues_dict['current_ready_for_testing']
    non_current_ready = issues_dict['non_current_ready_for_testing']
    current_testing = issues_dict['current_testing']
    non_current_testing = issues_dict['non_current_testing']

    print(f"Number of 'Ready For Testing' issues for current sprint: {len(current_ready)}")
    print(f"Number of 'Ready For Testing' issues for non-current sprint: {len(non_current_ready)}")
    print(f"Number of 'Testing' issues for current sprint: {len(current_testing)}")
    print(f"Number of 'Testing' issues for non-current sprint: {len(non_current_testing)}")

    send_slack_notification(user_mapping, issues_dict,
                            group_id, group_name, issue_template_url,
                            slack_bot_token, slack_channel)


if __name__ == "__main__":
    if len(sys.argv) < 4:
        print('Usage: python scan-and-notify-testing-items.py ' +
              '<github_org> <github_repo> <github_project>')
        sys.exit(1)

    scan_and_notify(sys.argv[1], sys.argv[2], sys.argv[3])
