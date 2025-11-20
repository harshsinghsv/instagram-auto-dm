# Testing Instagram Auto-DM with Meta Test Users

Meta's 24-hour messaging rule prevents sending DMs to users unless they've messaged you in the last 24 hours. For development and testing, use **test users** instead.

## Why You're Getting 403 Error

Error Code: `10` with subcode `2534022`

**Meaning:** "This message cannot be sent at this time" - You've hit Instagram's 24-hour messaging window restriction.

## Solution: Use Meta Test Users

### Step 1: Create Test Users

1. Go to your **Facebook App Dashboard**
2. Click **Tools** → **Roles** → **Test Users**
3. Click **Create Test User**
4. Fill in:
   - Name: `Test Instagram User`
   - Email: Auto-generated
   - Role: `Admin` or `App Tester`
5. Click **Create Test User**

### Step 2: Add Test User to App Roles

Each test user needs access to your app:

1. In **Roles**, find the test user
2. Click the user → **Edit**
3. Assign role: `Admin` or `App Tester`
4. Save

### Step 3: Install App for Test User

1. Go to **Settings** → **Basic**
2. Copy your **App ID** and **App Secret**
3. Test user will have access to your Instagram Business Account in test mode

### Step 4: Test Comment Flow

1. **Log in as test user** to Instagram (use test user credentials)
2. **Comment on your post** with a keyword (help, dm, info)
3. **Watch server logs** - should now send DM successfully

### Step 5: Verify DM Was Sent

Check your Instagram inbox - you should see the test user's message and your automated reply.

## Environment for Test Users

Your `.env` is already configured. Test users will use the same:
- `IG_BUSINESS_ID` (your account)
- `ACCESS_TOKEN` (must have test user permissions)

## Alternative: Ask Friend to Message First

If you want to test with a real account:

1. **Have a real user message you directly** on Instagram
2. This opens a 24-hour messaging window
3. Now you can reply/send DM to them
4. Your webhook comment trigger will work for 24 hours after their message

## Production Deployment

In production, the 24-hour rule is important:

- **Allowed:** Replying to comments in real conversations
- **Allowed:** Sending DMs to users who've messaged you in last 24 hours
- **Not allowed:** Unsolicited mass DMs

Your app respects this by only sending DMs to users who comment (user-initiated), which falls within Meta's acceptable use policy for many regions.

## Check Permissions

Verify your access token has the right permissions:

```bash
curl -X GET "https://graph.instagram.com/me/permissions?access_token=YOUR_TOKEN"
```

Should include:
- `instagram_basic`
- `instagram_manage_messages`
- `pages_messaging` (sometimes needed)

## Still Not Working?

1. **Verify token:** Test with cURL directly
2. **Check app status:** App Review → Check for approval/restrictions
3. **Use sandbox mode:** Go to Settings → Development → Sandbox
4. **Check error subcode:** 
   - `2534022` = 24-hour window expired
   - `190` = Invalid token
   - `200` = Permissions error

## Rate Limits

Meta also has rate limits (429 errors). Current code handles this with exponential backoff.

If you see many 429 errors:
- Space out DM_DELAY (currently 1s)
- Consider queue rate limiting
- Contact Meta for higher limits
