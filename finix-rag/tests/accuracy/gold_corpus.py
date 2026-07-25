"""Gold corpus + Q&A set for FINIX RAG accuracy evaluation.

Five synthetic Indian-banking documents with unambiguous facts, plus a labelled
question set. Kept deliberately factual (RBI/Basel/KYC style) so answers can be
scored by exact keyword presence. Documents are multi-paragraph on purpose so some
produce >1 chunk — this also exercises the per-chunk integrity-hash fix.
"""

# Each doc: the first line is the title (DocumentProcessor uses it as the citation
# source). `hint` is a distinctive substring of the title used to score retrieval.
CORPUS = [
    {
        "filename": "npa_asset_quality_q4fy25.pdf",
        "hint": "NPA",
        "body": (
            "NPA and Asset Quality Report - Q4 FY2025\n\n"
            "This report summarises the asset quality position of the bank as of "
            "31 March 2025. The Gross NPA ratio stood at 3.2 percent, an improvement "
            "of 40 basis points over the previous quarter. The Net NPA ratio was "
            "0.8 percent.\n\n"
            "The Provision Coverage Ratio, including technical write-offs, improved to "
            "78.5 percent. Slippages during the quarter were contained at 1.1 percent "
            "of standard advances. The bank expects further improvement in recoveries "
            "from the National Company Law Tribunal resolution pipeline in FY2026."
        ),
    },
    {
        "filename": "capital_adequacy_basel3.pdf",
        "hint": "Capital",
        "body": (
            "Capital Adequacy and Basel III Disclosure\n\n"
            "As of the reporting date, the bank's Capital to Risk-weighted Assets Ratio "
            "(CRAR) under Basel III norms was 16.4 percent, well above the regulatory "
            "minimum of 11.5 percent including the capital conservation buffer.\n\n"
            "The Common Equity Tier 1 (CET1) ratio was 13.1 percent and the Tier 1 "
            "capital ratio was 14.2 percent. The bank maintained a Liquidity Coverage "
            "Ratio (LCR) of 142 percent, comfortably above the 100 percent requirement."
        ),
    },
    {
        "filename": "kyc_aml_compliance_circular.pdf",
        "hint": "KYC",
        "body": (
            "KYC and AML Compliance Circular\n\n"
            "Customer Due Diligence (CDD) must be completed within 30 days of account "
            "opening. Accounts where CDD is not completed within the stipulated period "
            "shall be subject to restrictions on further transactions.\n\n"
            "Periodic KYC updation is required every 2 years for high-risk customers, "
            "every 8 years for medium-risk customers and every 10 years for low-risk "
            "customers. Cash transactions above Rupees 10 lakh must be reported as a "
            "Cash Transaction Report (CTR) to FIU-IND by the fifteenth of the following "
            "month."
        ),
    },
    {
        "filename": "priority_sector_lending.pdf",
        "hint": "Priority",
        "body": (
            "Priority Sector Lending Guidelines\n\n"
            "Scheduled commercial banks are required to allocate 40 percent of their "
            "Adjusted Net Bank Credit (ANBC) to priority sectors. Compliance is assessed "
            "on a quarterly average basis.\n\n"
            "Within the overall target, the sub-target for Agriculture is 18 percent of "
            "ANBC, and the sub-target for Micro Enterprises is 7.5 percent of ANBC. "
            "Shortfalls in priority sector lending are deposited with the Rural "
            "Infrastructure Development Fund maintained by NABARD."
        ),
    },
    {
        "filename": "digital_banking_upi_ops.pdf",
        "hint": "Digital",
        "body": (
            "Digital Banking and UPI Operations\n\n"
            "The standard UPI transaction limit for person-to-person and person-to-"
            "merchant payments is Rupees 1 lakh per transaction. For specified "
            "categories such as capital markets, insurance and verified merchants, the "
            "enhanced UPI limit is Rupees 2 lakh per transaction.\n\n"
            "The bank's mobile application supports biometric login, 24x7 IMPS transfers "
            "and UPI AutoPay mandates. Failed UPI transactions are auto-reversed within "
            "the timelines prescribed by NPCI, failing which penalty is credited to the "
            "customer."
        ),
    },
]

# Each item: question, list of expected keywords (ALL must appear in the answer,
# case-insensitive), and the retrieval `hint` of the document that should be cited.
GOLD_QA = [
    {"q": "What was the gross NPA ratio for Q4 FY2025?", "keywords": ["3.2"], "hint": "NPA"},
    {"q": "What is the net NPA ratio?", "keywords": ["0.8"], "hint": "NPA"},
    {"q": "What is the provision coverage ratio?", "keywords": ["78.5"], "hint": "NPA"},
    {"q": "What was the bank's capital adequacy ratio (CRAR) under Basel III?", "keywords": ["16.4"], "hint": "Capital"},
    {"q": "What is the CET1 ratio?", "keywords": ["13.1"], "hint": "Capital"},
    {"q": "What liquidity coverage ratio did the bank maintain?", "keywords": ["142"], "hint": "Capital"},
    {"q": "Within how many days must customer due diligence be completed?", "keywords": ["30"], "hint": "KYC"},
    {"q": "How often is periodic KYC update required for high-risk customers?", "keywords": ["2"], "hint": "KYC"},
    {"q": "What percentage of ANBC must banks lend to priority sectors?", "keywords": ["40"], "hint": "Priority"},
    {"q": "What is the agriculture sub-target for priority sector lending?", "keywords": ["18"], "hint": "Priority"},
    {"q": "What is the standard UPI transaction limit per transaction?", "keywords": ["1 lakh"], "hint": "Digital"},
    {"q": "What is the enhanced UPI limit for capital markets and insurance?", "keywords": ["2 lakh"], "hint": "Digital"},
]
