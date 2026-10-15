# Only pending invitations can be imported. invite_url stays empty until the link is reissued, and the
# first apply after import records send_email and resend_trigger without calling the API.
terraform import opstracer_org_invitation.alice <invitation-id>
