{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Resource": "*",
      "Action": [
        "codecommit:*"
      ]
    },
    {
      "Effect": "Deny",
      "Resource": "*",
      "Action": [
        "codecommit:ListApprovalRuleTemplates"
      ]
    }
  ]
}
